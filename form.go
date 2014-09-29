package restclient

import (
	"net/url"
	"reflect"
	"strconv"
	"strings"

	"github.com/golang/glog"
)

type tagOptions string

// encodeForm encodes the request body to url.Values.Encode()
func (r *Request) encodeForm() ([]byte, error) {
	var out []byte
	if glog.V(3) {
		glog.Infoln("Encoding bodyObject (", r.RequestBody, ") to url.Values form")
	}

	v, err := structToVals(r.RequestBody)
	if err != nil {
		glog.Errorln("Failed to convert struct to url.Values:", err.Error())
		return out, err
	}

	// Convert the Request struct to an url.Values map
	// Encode url.Values to urlencoded format
	out = []byte(v.Encode())
	return out, nil
}

// Convert a struct to an url.Values map
func structToVals(s interface{}) (url.Values, error) {
	v := url.Values{}
	structVals := reflect.ValueOf(s).Elem()
	t := structVals.Type()
	for i := 0; i < structVals.NumField(); i++ {
		f := structVals.Field(i)
		var val string
		switch f.Interface().(type) {
		case int, int8, int16, int32, int64:
			val = strconv.FormatInt(f.Int(), 10)
		case uint, uint8, uint16, uint32, uint64:
			val = strconv.FormatUint(f.Uint(), 10)
		case float32:
			val = strconv.FormatFloat(f.Float(), 'f', 4, 32)
		case float64:
			val = strconv.FormatFloat(f.Float(), 'f', 4, 64)
		case []byte:
			val = string(f.Bytes())
		case string:
			val = f.String()
		default:
			glog.Warningln("Ignoring unhandled type")
			continue
		}
		name, opts := getTagName(t.Field(i))
		if name == "" {
			// If we have no name, ignore this field
			continue
		}
		// Check for omitempty
		if val == "" && opts.Contains("omitempty") {
			// If we have no value and we have omitempty set, ignore this field
			continue
		}

		v.Set(name, val)
	}
	return v, nil
}

// getTagName returns the name from the tag and a list of
// options (such as omitempty)
func getTagName(f reflect.StructField) (string, tagOptions) {
	var name string
	var opts tagOptions

	// Check for the 'form' tag preferentially
	tagText := f.Tag.Get("form")
	if tagText != "" {
		// Explicit ignore
		if tagText == "-" {
			return "", ""
		}
		// Extract options
		name = tagText
		if index := strings.Index(tagText, ","); index != -1 {
			name = tagText[:index]
			opts = tagOptions(tagText[index+1:])
		}
	} else if tagText = f.Tag.Get("json"); tagText != "" {
		// Explicit ignore
		if tagText == "-" {
			return "", ""
		}
		// Extract options
		name = tagText
		if index := strings.Index(tagText, ","); index != -1 {
			name = tagText[:index]
			opts = tagOptions(tagText[index+1:])
		}
	} else {
		name = f.Name
	}
	return name, opts
}

func (o tagOptions) Contains(optionName string) bool {
	if len(o) == 0 {
		return false
	}
	s := string(o)
	for s != "" {
		var next string
		i := strings.Index(s, ",")
		if i >= 0 {
			s, next = s[:i], s[i+1:]
		}
		if s == optionName {
			return true
		}
		s = next
	}
	return false
}
