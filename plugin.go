package gendoc

import (
	"fmt"
	"io/ioutil"
	"path"
	"regexp"
	"strings"

	"github.com/golang/protobuf/proto"
	plugin_go "github.com/golang/protobuf/protoc-gen-go/plugin"
	"github.com/kouzoh/mercari-data/src/go/internal/events/core"
	"github.com/kouzoh/mercari-data/src/go/pkg/protoutils"
	_ "github.com/kouzoh/mercari-data/src/proto/event/v1beta/client" // imported for side effects
	"github.com/pseudomuto/protokit"
)

// PluginOptions encapsulates options for the plugin. The type of renderer, template file, and the name of the output
// file are included.
type PluginOptions struct {
	Type            RenderType
	TemplateFile    string
	OutputFile      string
	ExcludePatterns []*regexp.Regexp
}

// Plugin describes a protoc code generate plugin. It's an implementation of Plugin from github.com/pseudomuto/protokit
type Plugin struct{}

// Generate compiles the documentation and generates the CodeGeneratorResponse to send back to protoc. It does this
// by rendering a template based on the options parsed from the CodeGeneratorRequest.
func (p *Plugin) Generate(r *plugin_go.CodeGeneratorRequest) (*plugin_go.CodeGeneratorResponse, error) {
	options, err := ParseOptions(r)
	if err != nil {
		return nil, err
	}

	result := excludeUnwantedProtos(protokit.ParseCodeGenRequest(r), options.ExcludePatterns)
	template := NewTemplate(result)

	customTemplate := ""

	if options.TemplateFile != "" {
		data, err := ioutil.ReadFile(options.TemplateFile)
		if err != nil {
			return nil, err
		}

		customTemplate = string(data)
	}

	output, err := RenderTemplate(options.Type, template, customTemplate)
	if err != nil {
		return nil, err
	}

	resp := new(plugin_go.CodeGeneratorResponse)
	resp.File = append(resp.File, &plugin_go.CodeGeneratorResponse_File{
		Name:    proto.String(options.OutputFile),
		Content: proto.String(string(output)),
	})

	return resp, nil
}

func newTemplateWithAnnotations(descs []*protokit.FileDescriptor) *Template {
	template := NewTemplate(descs)
	addEnumOptionsToTemplate(template)

	return template
}

func addEnumOptionsToTemplate(template *Template) {
	for _, f := range template.Files {
		if f.HasEnums {
			for _, e := range f.Enums {
				fr, err := getFieldRequirementsMap(f.Name, e.Name)
				if err != nil {
					return
				}

				// If we have field requirements for this enum, stick them in
				// the options and add a flag to the enum's options.
				if len(fr) != 0 {
					if e.Options == nil {
						e.Options = make(map[string]interface{})
					}
					e.Options["has_field_requirements"] = true

					for _, v := range e.Values {
						if v.Options == nil {
							v.Options = make(map[string]interface{})
						}
						v.Options["requirements"] = fr
					}
				}
			}
		}
	}
}

func getFieldRequirementsMap(protoFileName, enumName string) (map[string][]*core.FieldRequirements, error) {
	annotations, err := protoutils.ExtractAnnotations(protoFileName, enumName)
	if err != nil {
		return nil, fmt.Errorf("can't get annotations for %s:%s, %v", protoFileName, enumName, err)
	}

	return core.CreateFieldRequirementsMap(annotations), nil
}

// func getFieldRequirementsMap(protoFileName, enumName string) (map[string][]*interface{}, error) {
// 	return make(map[string][]*interface{}), nil
// }

func excludeUnwantedProtos(fds []*protokit.FileDescriptor, excludePatterns []*regexp.Regexp) []*protokit.FileDescriptor {
	descs := make([]*protokit.FileDescriptor, 0)

OUTER:
	for _, d := range fds {
		for _, p := range excludePatterns {
			if p.MatchString(d.GetName()) {
				continue OUTER
			}
		}

		descs = append(descs, d)
	}

	return descs
}

// ParseOptions parses plugin options from a CodeGeneratorRequest. It does this by splitting the `Parameter` field from
// the request object and parsing out the type of renderer to use and the name of the file to be generated.
//
// The parameter (`--doc_opt`) must be of the format <TYPE|TEMPLATE_FILE>,<OUTPUT_FILE>:<EXCLUDE_PATTERN>,<EXCLUDE_PATTERN>*.
// The file will be written to the directory specified with the `--doc_out` argument to protoc.
func ParseOptions(req *plugin_go.CodeGeneratorRequest) (*PluginOptions, error) {
	options := &PluginOptions{
		Type:         RenderTypeHTML,
		TemplateFile: "",
		OutputFile:   "index.html",
	}

	params := req.GetParameter()
	if strings.Contains(params, ":") {
		// Parse out exclude patterns if any
		parts := strings.Split(params, ":")
		for _, pattern := range strings.Split(parts[1], ",") {
			r, err := regexp.Compile(pattern)
			if err != nil {
				return nil, err
			}
			options.ExcludePatterns = append(options.ExcludePatterns, r)
		}
		// The first part is parsed below
		params = parts[0]
	}
	if params == "" {
		return options, nil
	}

	if !strings.Contains(params, ",") {
		return nil, fmt.Errorf("Invalid parameter: %s", params)
	}

	parts := strings.Split(params, ",")
	if len(parts) != 2 {
		return nil, fmt.Errorf("Invalid parameter: %s", params)
	}

	options.TemplateFile = parts[0]
	options.OutputFile = path.Base(parts[1])

	renderType, err := NewRenderType(options.TemplateFile)
	if err == nil {
		options.Type = renderType
		options.TemplateFile = ""
	}

	return options, nil
}
