package search

import (
	"fmt"
	"reflect"
	"strings"

	v1 "github.com/stackrox/rox/generated/api/v1"
	"github.com/stackrox/rox/pkg/protoreflect"
	"github.com/stackrox/rox/pkg/search/enumregistry"
)

type PathElem struct {
	Name string
	Slice bool
}

func deepCopyPathElems(o []PathElem) []PathElem {
	copied := make([]PathElem, 0, len(o))
	for _, e := range o {
		copied = append(copied, e)
	}
	return copied
}

type searchWalker struct {
	prefix string
	category v1.SearchCategory
	fields   map[FieldLabel]*Field
}

func (s *searchWalker) elemsToPath(elems []PathElem) string {
	var names []string
	names = append(names, s.prefix)
	for _, e := range elems {
		names = append(names, e.Name)
	}
	return strings.Join(names, ".")
}

// Walk iterates over the obj and creates a search.Map object from the found struct tags
func Walk(category v1.SearchCategory, prefix string, obj interface{}) OptionsMap {
	sw := searchWalker{
		prefix: prefix,
		category: category,
		fields:   make(map[FieldLabel]*Field),
	}
	var pathElems []PathElem
	sw.walkRecursive(pathElems, reflect.TypeOf(obj))
	return OptionsMapFromMap(category, sw.fields)
}

func (s *searchWalker) getSearchField(path, tag string) (string, *Field) {
	if tag == "" {
		return "", nil
	}
	fields := strings.Split(tag, ",")
	if !FieldLabelSet.Contains(fields[0]) {
		log.Panicf("Field %q is not a valid FieldLabel. You may need to add it to pkg/search/options.go", fields[0])
	}

	fieldName := fields[0]
	var hidden, store bool
	var analyzer string
	if len(fields) > 1 {
		for _, f := range fields[1:] {
			switch f {
			case "hidden":
				hidden = true
			case "store":
				store = true
			default:
				if strings.HasPrefix(f, "analyzer=") {
					spl := strings.Split(f, "=")
					if len(spl) != 2 {
						log.Fatalf("Invalid analyzer struct annotation %q on field %q", f, fieldName)
					}
					analyzer = spl[1]
				} else {
					log.Fatalf("Field %s in search annotation is invalid", f)
				}
			}
		}
	}

	return fieldName, &Field{
		FieldPath: path,
		Store:     store,
		Hidden:    hidden,
		Category:  s.category,
		Analyzer:  analyzer,
	}
}

// handleStruct takes in a struct object and properly handles all of the fields
func (s *searchWalker) handleStruct(parentElems []PathElem, original reflect.Type) {
	for i := 0; i < original.NumField(); i++ {

		field := original.Field(i)
		jsonTag := strings.TrimSuffix(field.Tag.Get("json"), ",omitempty")
		if jsonTag == "-" {
			continue
		} else if jsonTag == "" { // If no JSON tag, then Bleve takes the field name
			jsonTag = field.Name
		}

		searchTag := field.Tag.Get("search")
		if searchTag == "-" {
			continue
		}
		pathElems := deepCopyPathElems(parentElems)

		pathElems = append(pathElems, PathElem{
			Name: jsonTag,
			Slice: field.Type.Kind() == reflect.Slice,
		})
		fmt.Println(s.elemsToPath(pathElems), field.Type.Kind().String())

		fullPath := s.elemsToPath(pathElems)
		// Special case proto timestamp because we actually want to index seconds
		if field.Type.String() == "*types.Timestamp" {
			fieldName, searchField := s.getSearchField(fullPath+".seconds", searchTag)
			if searchField == nil {
				continue
			}
			searchField.Type = v1.SearchDataType_SEARCH_DATETIME
			searchField.Elems = pathElems
			s.fields[FieldLabel(fieldName)] = searchField
			continue
		}
		// If it is a oneof then call XXX_OneofWrappers to get the types.
		// The return values is a slice of interfaces that are nil type pointers
		if field.Tag.Get("protobuf_oneof") != "" {
			ptrToOriginal := reflect.PtrTo(original)

			methodName := fmt.Sprintf("Get%s", field.Name)
			oneofGetter, ok := ptrToOriginal.MethodByName(methodName)
			if !ok {
				panic("didn't find oneof function, did the naming change?")
			}
			oneofInterfaces := oneofGetter.Func.Call([]reflect.Value{reflect.New(original)})
			if len(oneofInterfaces) != 1 {
				panic(fmt.Sprintf("found %d interfaces returned from oneof getter", len(oneofInterfaces)))
			}

			oneofInterface := oneofInterfaces[0].Type()

			method, ok := ptrToOriginal.MethodByName("XXX_OneofWrappers")
			if !ok {
				panic(fmt.Sprintf("XXX_OneofWrappers should exist for all protobuf oneofs, not found for %s", original.Name()))
			}
			out := method.Func.Call([]reflect.Value{reflect.New(original)})
			actualOneOfFields := out[0].Interface().([]interface{})
			for _, f := range actualOneOfFields {
				typ := reflect.TypeOf(f)
				if typ.Implements(oneofInterface) {
					s.walkRecursive(pathElems, typ)
				}
			}
			continue
		}

		searchDataType := s.walkRecursive(pathElems, field.Type)
		fieldName, searchField := s.getSearchField(fullPath, searchTag)
		if searchField == nil {
			continue
		}
		searchField.Type = searchDataType
		searchField.Elems = pathElems
		s.fields[FieldLabel(fieldName)] = searchField
	}
}

func (s *searchWalker) walkRecursive(pathElems []PathElem, original reflect.Type) v1.SearchDataType {
	switch original.Kind() {
	case reflect.Ptr, reflect.Slice:
		return s.walkRecursive(pathElems, original.Elem())
	case reflect.Struct:
		s.handleStruct(pathElems, original)
	case reflect.Map:
		return v1.SearchDataType_SEARCH_MAP
	case reflect.String:
		return v1.SearchDataType_SEARCH_STRING
	case reflect.Bool:
		return v1.SearchDataType_SEARCH_BOOL
	case reflect.Uint32, reflect.Uint64, reflect.Int32, reflect.Int64, reflect.Float32, reflect.Float64:
		enum, ok := reflect.Zero(original).Interface().(protoreflect.ProtoEnum)
		if !ok {
			return v1.SearchDataType_SEARCH_NUMERIC
		}
		enumDesc, err := protoreflect.GetEnumDescriptor(enum)
		if err != nil {
			panic(err)
		}
		enumregistry.Add(s.elemsToPath(pathElems), enumDesc)
		return v1.SearchDataType_SEARCH_ENUM
	case reflect.Interface:
	default:
		panic(fmt.Sprintf("Type %s for field %s is not currently handled", original.Kind(), s.elemsToPath(pathElems)))
	}
	return v1.SearchDataType_SEARCH_STRING
}
