package binding

import (
	"bytes"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"text/template"
	"unicode"

	"github.com/nspcc-dev/neo-go/pkg/smartcontract"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/callflag"
	"github.com/nspcc-dev/neo-go/pkg/smartcontract/manifest"
	"github.com/nspcc-dev/neo-go/pkg/util"
)

const srcTmpl = `
{{- define "METHOD" -}}
// {{.Name}} {{.Comment}}
func {{.Name}}({{range $index, $arg := .Arguments -}}
	{{- if ne $index 0}}, {{end}}
		{{- .Name}} {{.Type}}
	{{- end}}) {{if .ReturnType }}{{ .ReturnType }} {
	return neogointernal.CallWithToken(Hash, "{{ .NameABI }}", int(contract.{{ .CallFlag }})
		{{- range $arg := .Arguments -}}, {{.Name}}{{end}}).({{ .ReturnType }})
	{{- else -}} {
	neogointernal.CallWithTokenNoRet(Hash, "{{ .NameABI }}", int(contract.{{ .CallFlag }})
		{{- range $arg := .Arguments -}}, {{.Name}}{{end}})
	{{- end}}
}
{{- end -}}
// Package {{.PackageName}} contains wrappers for {{.ContractName}} contract.
package {{.PackageName}}

import (
{{range $m := .Imports}}	"{{ $m }}"
{{end}})

// Hash contains contract hash in big-endian form.
const Hash = "{{ .Hash }}"
{{range $m := .Methods}}
{{template "METHOD" $m }}
{{end}}`

type (
	// Config contains parameter for the generated binding.
	Config struct {
		Package   string                       `yaml:"package,omitempty"`
		Manifest  *manifest.Manifest           `yaml:"-"`
		Hash      util.Uint160                 `yaml:"hash,omitempty"`
		Overrides map[string]Override          `yaml:"overrides,omitempty"`
		CallFlags map[string]callflag.CallFlag `yaml:"callflags,omitempty"`
		Output    io.Writer                    `yaml:"-"`
	}

	contractTmpl struct {
		PackageName  string
		ContractName string
		Imports      []string
		Hash         string
		Methods      []methodTmpl
	}

	methodTmpl struct {
		Name       string
		NameABI    string
		CallFlag   string
		Comment    string
		Arguments  []paramTmpl
		ReturnType string
	}

	paramTmpl struct {
		Name string
		Type string
	}
)

// NewConfig initializes and returns a new config instance.
func NewConfig() Config {
	return Config{
		Overrides: make(map[string]Override),
		CallFlags: make(map[string]callflag.CallFlag),
	}
}

// Generate writes Go file containing smartcontract bindings to the `cfg.Output`.
func Generate(cfg Config) error {
	ctr, err := templateFromManifest(cfg)
	if err != nil {
		return err
	}

	tmp, err := template.New("generate").Funcs(template.FuncMap{
		"lowerFirst": lowerFirst,
		"scTypeToGo": scTypeToGo,
	}).Parse(srcTmpl)
	if err != nil {
		return err
	}

	return tmp.Execute(cfg.Output, ctr)
}

func scTypeToGo(typ smartcontract.ParamType) string {
	switch typ {
	case smartcontract.AnyType:
		return "interface{}"
	case smartcontract.BoolType:
		return "bool"
	case smartcontract.IntegerType:
		return "int"
	case smartcontract.ByteArrayType:
		return "[]byte"
	case smartcontract.StringType:
		return "string"
	case smartcontract.Hash160Type:
		return "interop.Hash160"
	case smartcontract.Hash256Type:
		return "interop.Hash256"
	case smartcontract.PublicKeyType:
		return "interop.PublicKey"
	case smartcontract.SignatureType:
		return "interop.Signature"
	case smartcontract.ArrayType:
		return "[]interface{}"
	case smartcontract.MapType:
		return "map[string]interface{}"
	case smartcontract.InteropInterfaceType:
		return "interface{}"
	case smartcontract.VoidType:
		return ""
	default:
		panic("unreachable")
	}
}

func templateFromManifest(cfg Config) (contractTmpl, error) {
	hStr := ""
	for _, b := range cfg.Hash.BytesBE() {
		hStr += fmt.Sprintf("\\x%02x", b)
	}

	ctr := contractTmpl{
		PackageName:  cfg.Package,
		ContractName: cfg.Manifest.Name,
		Hash:         hStr,
	}
	if ctr.PackageName == "" {
		buf := bytes.NewBuffer(make([]byte, 0, len(cfg.Manifest.Name)))
		for _, r := range cfg.Manifest.Name {
			if unicode.IsLetter(r) {
				buf.WriteRune(unicode.ToLower(r))
			}
		}

		ctr.PackageName = buf.String()
	}

	imports := make(map[string]struct{})
	seen := make(map[string]bool)
	for _, m := range cfg.Manifest.ABI.Methods {
		seen[m.Name] = false
	}
	for _, m := range cfg.Manifest.ABI.Methods {
		if m.Name[0] == '_' {
			continue
		}

		imports["github.com/nspcc-dev/neo-go/pkg/interop/contract"] = struct{}{}
		imports["github.com/nspcc-dev/neo-go/pkg/interop/neogointernal"] = struct{}{}

		// Consider `perform(a)` and `perform(a, b)` methods.
		// First, try to export the second method with `Perform2` name.
		// If `perform2` is already in the manifest, use `perform_2` with as many underscores
		// as needed to eliminate name conflicts. It will produce long names in certain circumstances,
		// but if the manifest contains lots of similar names with trailing underscores, delicate naming
		// was probably not the goal.
		name := m.Name
		if v, ok := seen[name]; !ok || v {
			suffix := strconv.Itoa(len(m.Parameters))
			for ; seen[name]; name = m.Name + suffix {
				suffix = "_" + suffix
			}
		}
		seen[name] = true

		mtd := methodTmpl{
			Name:     upperFirst(name),
			NameABI:  m.Name,
			CallFlag: callflag.All.String(),
			Comment:  fmt.Sprintf("invokes `%s` method of contract.", m.Name),
		}
		if f, ok := cfg.CallFlags[m.Name]; ok {
			mtd.CallFlag = f.String()
		} else if m.Safe {
			mtd.CallFlag = callflag.ReadOnly.String()
		}
		for i := range m.Parameters {
			name := m.Parameters[i].Name
			if name == "" {
				name = fmt.Sprintf("arg%d", i)
			}

			var typeStr string
			if over, ok := cfg.Overrides[m.Name+"."+name]; ok {
				typeStr = over.TypeName
				if over.Package != "" {
					imports[over.Package] = struct{}{}
				}
			} else {
				typeStr = scTypeToGo(m.Parameters[i].Type)
			}

			mtd.Arguments = append(mtd.Arguments, paramTmpl{
				Name: name,
				Type: typeStr,
			})
		}

		if over, ok := cfg.Overrides[m.Name]; ok {
			mtd.ReturnType = over.TypeName
			if over.Package != "" {
				imports[over.Package] = struct{}{}
			}
		} else {
			mtd.ReturnType = scTypeToGo(m.ReturnType)
			switch m.ReturnType {
			case smartcontract.Hash160Type, smartcontract.Hash256Type, smartcontract.InteropInterfaceType,
				smartcontract.SignatureType, smartcontract.PublicKeyType:
				imports["github.com/nspcc-dev/neo-go/pkg/interop"] = struct{}{}
			}
		}

		ctr.Methods = append(ctr.Methods, mtd)
	}

	for imp := range imports {
		ctr.Imports = append(ctr.Imports, imp)
	}
	sort.Strings(ctr.Imports)

	return ctr, nil
}

func upperFirst(s string) string {
	return strings.ToUpper(s[0:1]) + s[1:]
}

func lowerFirst(s string) string {
	return strings.ToLower(s[0:1]) + s[1:]
}
