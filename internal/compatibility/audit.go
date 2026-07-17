// Package compatibility compares published public contracts during a release.
package compatibility

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// Finding is a stable identifier for a structural breaking change.
type Finding struct{ ID string }

// Audit compares public OpenAPI and root SDK surfaces. Additions are ignored;
// only declarations present in base but absent from current are reported.
func Audit(baseOpenAPI, currentOpenAPI []byte, baseSDK, currentSDK map[string][]byte) ([]Finding, error) {
	base, err := loadOpenAPI(baseOpenAPI)
	if err != nil {
		return nil, fmt.Errorf("load base OpenAPI: %w", err)
	}
	current, err := loadOpenAPI(currentOpenAPI)
	if err != nil {
		return nil, fmt.Errorf("load current OpenAPI: %w", err)
	}
	findings := auditOpenAPI(base, current)
	baseSymbols, err := sdkSymbols(baseSDK)
	if err != nil {
		return nil, fmt.Errorf("parse base SDK: %w", err)
	}
	currentSymbols, err := sdkSymbols(currentSDK)
	if err != nil {
		return nil, fmt.Errorf("parse current SDK: %w", err)
	}
	for symbol := range baseSymbols {
		if _, ok := currentSymbols[symbol]; !ok {
			findings = append(findings, Finding{ID: "sdk.symbol_removed:" + symbol})
		}
	}
	return sorted(findings), nil
}

func loadOpenAPI(raw []byte) (*openapi3.T, error) {
	return openapi3.NewLoader().LoadFromData(raw)
}

func auditOpenAPI(base, current *openapi3.T) []Finding {
	findings := make([]Finding, 0)
	for path, baseItem := range base.Paths.Map() {
		currentItem := current.Paths.Value(path)
		if currentItem == nil {
			findings = append(findings, Finding{ID: "openapi.path_removed:" + path})
			continue
		}
		for method, baseOperation := range baseItem.Operations() {
			currentOperation := currentItem.GetOperation(method)
			if currentOperation == nil {
				findings = append(findings, Finding{ID: "openapi.operation_removed:" + strings.ToUpper(method) + " " + path})
				continue
			}
			for status := range baseOperation.Responses.Map() {
				if currentOperation.Responses.Value(status) == nil {
					findings = append(findings, Finding{ID: "openapi.response_removed:" + strings.ToUpper(method) + " " + path + " " + status})
				}
			}
		}
	}
	for name, baseSchema := range base.Components.Schemas {
		currentSchema := current.Components.Schemas[name]
		if currentSchema == nil || currentSchema.Value == nil {
			findings = append(findings, Finding{ID: "openapi.schema_removed:" + name})
			continue
		}
		findings = append(findings, auditSchema("openapi.schema_property_removed:"+name, baseSchema.Value, currentSchema.Value)...)
	}
	return findings
}

func auditSchema(prefix string, base, current *openapi3.Schema) []Finding {
	findings := make([]Finding, 0)
	for property, baseRef := range base.Properties {
		currentRef := current.Properties[property]
		if currentRef == nil || currentRef.Value == nil {
			findings = append(findings, Finding{ID: prefix + "." + property})
			continue
		}
		if baseRef != nil && baseRef.Value != nil {
			findings = append(findings, auditSchema(prefix+"."+property, baseRef.Value, currentRef.Value)...)
		}
	}
	return findings
}

func sdkSymbols(files map[string][]byte) (map[string]struct{}, error) {
	symbols := make(map[string]struct{})
	set := token.NewFileSet()
	for path, raw := range files {
		file, err := parser.ParseFile(set, path, raw, 0)
		if err != nil {
			return nil, err
		}
		if file.Name.Name != "orag" {
			continue
		}
		for _, declaration := range file.Decls {
			switch value := declaration.(type) {
			case *ast.FuncDecl:
				if value.Recv == nil && value.Name.IsExported() {
					symbols["func "+value.Name.Name] = struct{}{}
				}
				if value.Recv != nil && receiverName(value.Recv) == "Client" && value.Name.IsExported() {
					symbols["method Client."+value.Name.Name] = struct{}{}
				}
			case *ast.GenDecl:
				for _, spec := range value.Specs {
					switch spec := spec.(type) {
					case *ast.TypeSpec:
						if !spec.Name.IsExported() {
							continue
						}
						symbols["type "+spec.Name.Name] = struct{}{}
						if structure, ok := spec.Type.(*ast.StructType); ok {
							for _, field := range structure.Fields.List {
								for _, name := range field.Names {
									if name.IsExported() {
										symbols["field "+spec.Name.Name+"."+name.Name] = struct{}{}
									}
								}
							}
						}
					case *ast.ValueSpec:
						for _, name := range spec.Names {
							if name.IsExported() {
								kind := "var "
								if value.Tok == token.CONST {
									kind = "const "
								}
								symbols[kind+name.Name] = struct{}{}
							}
						}
					}
				}
			}
		}
	}
	return symbols, nil
}

func receiverName(fields *ast.FieldList) string {
	if fields == nil || len(fields.List) != 1 {
		return ""
	}
	switch typ := fields.List[0].Type.(type) {
	case *ast.Ident:
		return typ.Name
	case *ast.StarExpr:
		if name, ok := typ.X.(*ast.Ident); ok {
			return name.Name
		}
	}
	return ""
}

func sorted(findings []Finding) []Finding {
	sort.Slice(findings, func(i, j int) bool { return findings[i].ID < findings[j].ID })
	return findings
}
