package docs

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"

	cmds "github.com/ipfs/go-ipfs-cmds"
	"github.com/swaggest/openapi-go/openapi3"
)

// OpenAPIFormatter implements an OpenAPI generator. It is
// used to generate the IPFS OpenAPI schema.
type OpenAPIFormatter struct {
	reflector openapi3.Reflector
	spec      openapi3.Spec
	md        MarkdownFormatter
}

// FIXME Share this with markdown.go
var description = `When a Kubo IPFS node is running as a daemon, it exposes an HTTP RPC API that allows you to control the node and run the same commands you can from the command line.

In many cases, using this RPC API is preferable to embedding IPFS directly in your program — it allows you to maintain peer connections that are longer lived than your app and you can keep a single IPFS node running instead of several if your app can be launched multiple times. In fact, the ` + "`ipfs`" + ` CLI commands use this RPC API when operating in online mode.`

func (myself *OpenAPIFormatter) GenerateMetadata() {
	myself.reflector = openapi3.Reflector{}
	myself.reflector.Spec = &openapi3.Spec{Openapi: "3.0.0"}
	myself.reflector.Spec.Info.
		WithTitle("IPFS RPC API").
		WithVersion("0.13.0").
		WithDescription(description)
	myself.reflector.Spec.WithExternalDocs(openapi3.ExternalDocumentation{
		URL: "https://docs.ipfs.tech/reference/kubo/rpc/",
	})
	myself.spec = *myself.reflector.Spec
	myself.md = MarkdownFormatter{}
}

func genParameterForArgument(arg *Argument, aliasToArg bool) *openapi3.Parameter {
	var t openapi3.SchemaType
	switch arg.Type {
	case "bool":
		t = openapi3.SchemaTypeBoolean
	case "int", "uint", "int64":
		t = openapi3.SchemaTypeInteger
	case "string":
		t = openapi3.SchemaTypeString
	case "array":
		t = openapi3.SchemaTypeArray
	case "file":
		// This will be the request body.
		return nil
	default:
		log.Printf("WARN: Unsupported type for argument: %s\n", arg.Type)
		t = openapi3.SchemaTypeString
	}
	schema := openapi3.Schema{
		Type: &t,
	}
	if t == openapi3.SchemaTypeArray {
		t2 := openapi3.SchemaTypeString
		schema.Items = &openapi3.SchemaOrRef{
			Schema: &openapi3.Schema{
				Type: &t2,
			},
		}
	}
	if arg.Default != "" {
		var d any
		var err any
		switch t {
		case openapi3.SchemaTypeBoolean:
			d, err = strconv.ParseBool(arg.Default)
		case openapi3.SchemaTypeInteger:
			d, err = strconv.ParseInt(arg.Default, 10, 32)
		default:
			d = arg.Default
			err = nil
		}
		if err != nil {
			fmt.Println("WARN: Couldn't parse default value for " + arg.Name)
			d = arg.Default
		}
		schema.WithDefault(d)
	}
	alias := arg.Name
	if aliasToArg {
		alias = "arg"
	}
	description := strings.TrimSuffix(arg.Description, " Default: "+arg.Default+".")
	p := openapi3.Parameter{
		Name:        alias,
		In:          openapi3.ParameterInQuery,
		Description: &description,
		Schema:      &openapi3.SchemaOrRef{Schema: &schema},
		Content:     nil,
		//Required: &arg.Required,
	}
	if arg.Required {
		p.Required = &arg.Required
	}
	if strings.Contains(arg.Description, "(experimental)") {
		if p.MapOfAnything == nil {
			p.MapOfAnything = make(map[string]interface{})
		}
		p.MapOfAnything["x-experimental"] = true
	}
	if strings.Contains(arg.Description, "(DEPRECATED)") || strings.HasPrefix(arg.Description, "Removed, ") {
		d := true
		p.Deprecated = &d
	}
	return &p
}

func genParameterForMultiArgument(args []*Argument) *openapi3.Parameter {
	params := []*openapi3.Parameter{}
	defaults := []any{}
	anyDefault := false
	descriptions := []string{}
	deprecated := false
	required := false
	for i, arg := range args {
		p := genParameterForArgument(arg, false)
		d := "arg" + strconv.Itoa(i) + " (" + p.Name + "): " + strings.TrimSpace(*p.Description)
		p.Description = &d
		descriptions = append(descriptions, d)
		params = append(params, p)
		defaults = append(defaults, p.Schema.Schema.Default)
		anyDefault = anyDefault || p.Schema.Schema.Default != nil
		deprecated = deprecated || (p.Deprecated != nil && *p.Deprecated)
		required = p.Required != nil && *p.Required
	}

	t := openapi3.SchemaTypeArray
	t2 := openapi3.SchemaTypeString //FIXME use actual types of params
	num := int64(len(params))
	schema := openapi3.Schema{
		Type:     &t,
		MinItems: &num,
		MaxItems: &num,
		Items: &openapi3.SchemaOrRef{
			Schema: &openapi3.Schema{
				Type: &t2,
			},
		},
	}
	if anyDefault {
		schema.WithDefault(defaults)
	}
	alias := "arg"
	description := strings.Join(descriptions, "\n")
	e := true
	p := openapi3.Parameter{
		Name:        alias,
		In:          openapi3.ParameterInQuery,
		Description: &description,
		Schema:      &openapi3.SchemaOrRef{Schema: &schema},
		Content:     nil,
		//Required: &arg.Required,
		Explode: &e,
	}
	if required {
		p.Required = &required
	}
	if deprecated {
		p.Deprecated = &deprecated
	}
	return &p
}

func genSchemaForResponse(x any) *openapi3.Schema {
	switch v := x.(type) {
	case string:
		var t openapi3.SchemaType
		switch v {
		case "<bool>":
			t = openapi3.SchemaTypeBoolean
		case "<int>", "<uint>", "<int32>", "<uint32>", "<int64>", "<uint64>", "<duration-ns>", "<timestamp>":
			t = openapi3.SchemaTypeInteger
		case "<float32>", "<float64>":
			t = openapi3.SchemaTypeNumber
		case "<string>", "<peer-id>", "peer-id", "<cid-string>", "<multiaddr-string>":
			t = openapi3.SchemaTypeString
		case "<array>":
			t = openapi3.SchemaTypeArray
		case "<object>":
			t = openapi3.SchemaTypeObject
		default:
			log.Printf("WARN: Unsupported type for response: %s\n", v)
			return nil
		}
		schema := openapi3.Schema{
			Type: &t,
		}
		return &schema
	case []any:
		var itemType *openapi3.Schema
		if len(v) == 1 {
			itemType = genSchemaForResponse(v[0])
		}
		if itemType == nil {
			log.Println("WARN: Couldn't determine item type of array")
			itemType = &openapi3.Schema{} // allow any
		}
		t := openapi3.SchemaTypeArray
		schema := openapi3.Schema{
			Type:  &t,
			Items: &openapi3.SchemaOrRef{Schema: itemType},
		}
		return &schema
	case map[string]any:
		var firstKey string
		var firstValue any
		for k, v := range v {
			firstKey = k
			firstValue = v
			break
		}

		if len(v) == 1 && firstKey == "<string>" {
			var itemType *openapi3.Schema
			if len(v) == 1 {
				itemType = genSchemaForResponse(firstValue)
			}
			if itemType == nil {
				log.Println("WARN: Couldn't determine item type of object")
				itemType = &openapi3.Schema{} // allow any
			}

			t := openapi3.SchemaTypeObject
			schema := openapi3.Schema{
				Type: &t,
				AdditionalProperties: &openapi3.SchemaAdditionalProperties{
					SchemaOrRef: &openapi3.SchemaOrRef{Schema: itemType},
				},
			}
			return &schema
		} else {
			ps := map[string]openapi3.SchemaOrRef{}
			for k, v := range v {
				s := genSchemaForResponse(v)
				if s == nil {
					s = &openapi3.Schema{} // allow any
				}
				ps[k] = openapi3.SchemaOrRef{Schema: s}
			}
			t := openapi3.SchemaTypeObject
			schema := openapi3.Schema{
				Type:       &t,
				Properties: ps,
			}
			return &schema
		}
	default:
		log.Printf("WARN: Unsupported type for argument: %s\n", v)
		return nil
	}
}

func (myself *OpenAPIFormatter) GenerateEndpoint(endp *Endpoint) error {
	id := strings.TrimPrefix(endp.Name, "/api/v0/")
	refname := strings.Replace(strings.TrimPrefix(endp.Name, "/"), "/", "-", -1)
	op := openapi3.Operation{
		ID: &id,
		ExternalDocs: &openapi3.ExternalDocumentation{
			URL: "https://docs.ipfs.tech/reference/kubo/rpc/#" + refname,
		},
		Description: &endp.Description,
	}

	bodyArgs := []*Argument{}
	otherArgs := []*Argument{}
	for _, arg := range endp.Arguments {
		if arg.Type == "file" {
			bodyArgs = append(bodyArgs, arg)
		} else {
			otherArgs = append(otherArgs, arg)
		}
	}
	if len(otherArgs) > 1 {
		p := genParameterForMultiArgument(otherArgs)
		op.Parameters = append(op.Parameters, p.ToParameterOrRef())
	} else {
		//log.Println("FIXME: Special case for " + endp.Name + ": Multiple arguments `arg`. This should become an array.")
		for _, arg := range otherArgs {
			p := genParameterForArgument(arg, len(otherArgs) <= 1)
			op.Parameters = append(op.Parameters, p.ToParameterOrRef())
		}
	}
	for _, arg := range endp.Options {
		p := genParameterForArgument(arg, false)
		op.Parameters = append(op.Parameters, p.ToParameterOrRef())
	}

	if len(bodyArgs) > 0 {
		rb := openapi3.RequestBody{}

		// This spec uses the generated description, so let's do the same, for now.
		// https://app.swaggerhub.com/apis/powerpeaks/ipfs/1
		description := myself.md.GenerateBodyBlock(bodyArgs)
		description = strings.TrimSpace(description)
		description = strings.TrimPrefix(description, "### Request Body\n\n")
		rb.Description = &description

		object := openapi3.SchemaTypeObject
		array := openapi3.SchemaTypeArray
		string_t := openapi3.SchemaTypeString
		binary := "binary"
		rb.WithContentItem("multipart/form-data", openapi3.MediaType{
			// see https://swagger.io/docs/specification/describing-request-body/file-upload/
			Schema: &openapi3.SchemaOrRef{Schema: &openapi3.Schema{
				Type: &object,
				Properties: map[string]openapi3.SchemaOrRef{
					bodyArgs[0].Name: openapi3.SchemaOrRef{
						Schema: &openapi3.Schema{
							Type: &array,
							Items: &openapi3.SchemaOrRef{
								Schema: &openapi3.Schema{
									Type:   &string_t,
									Format: &binary,
								},
							},
						},
					},
				},
			}},
		})

		for _, arg := range bodyArgs {
			if arg.Required {
				rb.Required = &arg.Required
			}
		}
		op.WithRequestBody(openapi3.RequestBodyOrRef{RequestBody: &rb})
	}

	if endp.Response == "This endpoint returns a `text/plain` response body." {
		textBody := openapi3.MediaType{}
		resp := openapi3.Response{
			Description: "Successful response",
			Content: map[string]openapi3.MediaType{
				"text/plain": textBody,
			},
		}
		op.Responses.WithMapOfResponseOrRefValues(map[string]openapi3.ResponseOrRef{
			"200": {Response: &resp},
		})
	} else if endp.Response != "" {
		mimeJSON := "application/json"
		//var responseJson map[string]any
		var responseJson any
		err := json.Unmarshal([]byte(endp.Response), &responseJson)
		if err != nil {
			log.Println("Couldn't parse JSON for Response:", err, "; JSON:", endp.Response)
		} else {
			//log.Println("Response:", endp.Response)
			//example := map[string]string{}
			//example["bla"] = "blub"
			jsonBody := openapi3.MediaType{}
			jsonBody.WithExample(responseJson)

			schema := genSchemaForResponse(responseJson)
			if schema != nil {
				jsonBody.WithSchema(openapi3.SchemaOrRef{Schema: schema})
			}

			resp := openapi3.Response{
				Description: "Successful response",
				Content: map[string]openapi3.MediaType{
					mimeJSON: jsonBody,
				},
			}
			op.Responses.WithMapOfResponseOrRefValues(map[string]openapi3.ResponseOrRef{
				"200": {Response: &resp},
			})
		}
	}

	return myself.spec.AddOperation(http.MethodPost, endp.Name, op)
}

func (myself *OpenAPIFormatter) Generate(api []*Endpoint) error {
	myself.GenerateMetadata()

	for _, status := range []cmds.Status{cmds.Active, cmds.Experimental, cmds.Deprecated, cmds.Removed} {
		endpoints := InStatus(api, status)
		if len(endpoints) == 0 {
			continue
		}
		for _, endp := range endpoints {
			err := myself.GenerateEndpoint(endp)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

// GenerateDocs uses a formatter to generate documentation for every endpoint
func GenerateOpenAPI(api []*Endpoint, formatter OpenAPIFormatter) string {
	err := formatter.Generate(api)
	if err != nil {
		log.Fatal(err)
	}

	if false {
		schema, err := formatter.reflector.Spec.MarshalYAML()
		if err != nil {
			log.Fatal(err)
		}
		return string(schema)
	} else {
		schema, err := formatter.spec.MarshalYAML()
		if err != nil {
			log.Fatal(err)
		}
		return string(schema)
	}
}
