package rules

import (
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"
)

func videoSchema(t *testing.T) *ParamsSchema {
	t.Helper()
	s, err := CompileParamsSchema([]byte(videoSurveillance.ParamsSchema))
	if err != nil {
		t.Fatal(err)
	}

	return s
}

func TestValidateParams(t *testing.T) {
	s := videoSchema(t)

	valid := []string{
		`{"camera_count": 6, "payment_method": "management_bill"}`,
		`{"placement": "Входы в подъезды", "camera_count": 1000, "estimated_cost_rub": 0,
		  "payment_method": "special_assessment", "records_access": "house_council"}`,
		// JSON Schema: an integer is any number without a fractional part.
		`{"camera_count": 6.0, "payment_method": "management_bill"}`,
		`{"camera_count": 1e2, "payment_method": "management_bill"}`,
		// Length is counted in characters: 300 Cyrillic letters are 600 bytes.
		`{"camera_count": 1, "payment_method": "management_bill", "placement": "` + strings.Repeat("я", 300) + `"}`,
	}
	for _, params := range valid {
		if err := s.Validate([]byte(params)); err != nil {
			t.Errorf("%s: %v, want valid", params, err)
		}
	}

	invalid := []struct {
		params string
		field  string
		reason string
		limit  string
	}{
		{``, "camera_count", ParamsRequired, ""},
		{`null`, "camera_count", ParamsRequired, ""},
		{`{}`, "camera_count", ParamsRequired, ""},
		{`{"camera_count": 6}`, "payment_method", ParamsRequired, ""},
		{`[1, 2]`, "", ParamsNotObject, ""},
		{`"text"`, "", ParamsNotObject, ""},
		{`{"camera_count": 6} {}`, "", ParamsNotObject, ""},
		{`{"camera_count": 6, "payment_method": "management_bill", "owner_phone": "+7"}`, "owner_phone", ParamsUnknownField, ""},
		{`{"camera_count": "6", "payment_method": "management_bill"}`, "camera_count", ParamsType, ""},
		{`{"camera_count": 6.5, "payment_method": "management_bill"}`, "camera_count", ParamsType, ""},
		{`{"camera_count": null, "payment_method": "management_bill"}`, "camera_count", ParamsType, ""},
		{`{"camera_count": true, "payment_method": "management_bill"}`, "camera_count", ParamsType, ""},
		{`{"camera_count": 0, "payment_method": "management_bill"}`, "camera_count", ParamsMinimum, "1"},
		{`{"camera_count": 1001, "payment_method": "management_bill"}`, "camera_count", ParamsMaximum, "1000"},
		{`{"camera_count": 1, "payment_method": "cash"}`, "payment_method", ParamsEnum, ""},
		{`{"camera_count": 1, "payment_method": 1}`, "payment_method", ParamsType, ""},
		{`{"camera_count": 1, "payment_method": "management_bill", "estimated_cost_rub": -1}`, "estimated_cost_rub", ParamsMinimum, "0"},
		{`{"camera_count": 1, "payment_method": "management_bill", "placement": "` + strings.Repeat("я", 301) + `"}`,
			"placement", ParamsMaxLength, "300"},
	}
	for _, c := range invalid {
		err := s.Validate([]byte(c.params))
		var pe *ParamsError
		if !errors.As(err, &pe) || !errors.Is(err, ErrInvalidParams) {
			t.Errorf("%.80s: %v, want a ParamsError", c.params, err)

			continue
		}
		if pe.Field != c.field || pe.Reason != c.reason || pe.Limit != c.limit {
			t.Errorf("%.80s: field=%q reason=%q limit=%q, want %q %q %q", c.params, pe.Field, pe.Reason, pe.Limit,
				c.field, c.reason, c.limit)
		}
		if c.field != "" && c.reason != ParamsUnknownField && pe.Title == "" {
			t.Errorf("%.80s: no title of the field for the message", c.params)
		}
	}
}

// A number is not expanded exactly: a huge exponent must not cost memory or time.
func TestValidateParamsHugeNumbers(t *testing.T) {
	s := videoSchema(t)
	start := time.Now()
	for _, n := range []string{"1e999999999", "-1e999999999", "1e-999999999"} {
		err := s.Validate([]byte(`{"camera_count": ` + n + `, "payment_method": "management_bill"}`))
		var pe *ParamsError
		if !errors.As(err, &pe) || pe.Field != "camera_count" {
			t.Errorf("%s: %v, want an error of camera_count", n, err)
		}
	}
	if d := time.Since(start); d > time.Second {
		t.Fatalf("huge numbers took %s", d)
	}
}

func TestCompileParamsSchemaRejectsOutsideSubset(t *testing.T) {
	field := func(def string) string {
		return `{"type": "object", "additionalProperties": false, "properties": {"f": ` + def + `}}`
	}
	bad := map[string]string{
		"not an object":             `[]`,
		"not a JSON value":          `{"type": "object"`,
		"two JSON values":           `{"type": "object", "additionalProperties": false} {}`,
		"another draft":             `{"$schema": "http://json-schema.org/draft-07/schema#", "type": "object", "additionalProperties": false}`,
		"root is not an object":     `{"type": "array", "additionalProperties": false}`,
		"extra fields allowed":      `{"type": "object", "properties": {}}`,
		"extra fields true":         `{"type": "object", "additionalProperties": true}`,
		"unknown root keyword":      `{"type": "object", "additionalProperties": false, "oneOf": []}`,
		"required is not a field":   `{"type": "object", "additionalProperties": false, "required": ["x"], "properties": {}}`,
		"required twice":            `{"type": "object", "additionalProperties": false, "required": ["f", "f"], "properties": {"f": {"type": "boolean", "title": "F"}}}`,
		"unknown field keyword":     field(`{"type": "string", "title": "F", "maxLength": 5, "format": "email"}`),
		"unsupported type":          field(`{"type": "array", "title": "F"}`),
		"no type":                   field(`{"title": "F"}`),
		"no title":                  field(`{"type": "boolean"}`),
		"unbounded string":          field(`{"type": "string", "title": "F"}`),
		"numeric enum":              field(`{"type": "integer", "title": "F", "enum": ["1"]}`),
		"empty enum":                field(`{"type": "string", "title": "F", "enum": []}`),
		"repeated enum value":       field(`{"type": "string", "title": "F", "enum": ["a", "a"]}`),
		"enum with lengths":         field(`{"type": "string", "title": "F", "enum": ["a"], "maxLength": 3}`),
		"bounds on a string":        field(`{"type": "string", "title": "F", "maxLength": 3, "minimum": 1}`),
		"lengths on a number":       field(`{"type": "number", "title": "F", "maxLength": 3}`),
		"limits on a boolean":       field(`{"type": "boolean", "title": "F", "minimum": 0}`),
		"minimum above maximum":     field(`{"type": "integer", "title": "F", "minimum": 5, "maximum": 1}`),
		"min length above max":      field(`{"type": "string", "title": "F", "minLength": 5, "maxLength": 1}`),
		"negative length":           field(`{"type": "string", "title": "F", "maxLength": -1}`),
		"infinite bound":            field(`{"type": "number", "title": "F", "maximum": 1e400}`),
		"name is not an identifier": `{"type": "object", "additionalProperties": false, "properties": {"Camera Count": {"type": "boolean", "title": "F"}}}`,
	}
	for name, schema := range bad {
		if _, err := CompileParamsSchema([]byte(schema)); err == nil {
			t.Errorf("%s: compiled, want an error", name)
		}
	}

	// The smallest schema of the subset: a form without fields.
	s, err := CompileParamsSchema([]byte(`{"type": "object", "additionalProperties": false}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Validate(nil); err != nil {
		t.Fatalf("empty params of an empty form: %v", err)
	}
	if err := s.Validate([]byte(`{"x": 1}`)); err == nil {
		t.Fatal("a value of an empty form is accepted")
	}
}

// The mini-app builds the form from params_schema and ui_schema together: every field
// must be in the order, every enum value must have a title, the agenda must refer to
// known decision types.
func TestTemplatesAreConsistent(t *testing.T) {
	codes := map[string]bool{}
	for _, d := range decisionTypes {
		if _, err := ParseRule(d.Rule); err != nil {
			t.Errorf("decision type %s: %v", d.Code, err)
		}
		codes[d.Code] = true
	}

	for _, tpl := range templates {
		schema, err := CompileParamsSchema([]byte(tpl.ParamsSchema))
		if err != nil {
			t.Fatalf("%s: %v", tpl.Code, err)
		}

		var ui struct {
			Order      []string                     `json:"order"`
			Widgets    map[string]string            `json:"widgets"`
			EnumTitles map[string]map[string]string `json:"enum_titles"`
		}
		dec := json.NewDecoder(strings.NewReader(tpl.UISchema))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&ui); err != nil {
			t.Fatalf("%s: ui_schema: %v", tpl.Code, err)
		}

		if got := slices.Sorted(slices.Values(ui.Order)); !slices.Equal(got, schema.names) {
			t.Errorf("%s: order %v, want every field once: %v", tpl.Code, ui.Order, schema.names)
		}
		for name, widget := range ui.Widgets {
			if _, ok := schema.fields[name]; !ok || (widget != "textarea" && widget != "select") {
				t.Errorf("%s: widget %q of field %q", tpl.Code, widget, name)
			}
		}
		for _, name := range schema.names {
			f := schema.fields[name]
			titles := ui.EnumTitles[name]
			if f.Enum == nil {
				if titles != nil {
					t.Errorf("%s: enum_titles of %q, which is not an enum", tpl.Code, name)
				}

				continue
			}
			if got := slices.Sorted(maps.Keys(titles)); !slices.Equal(got, slices.Sorted(slices.Values(f.Enum))) {
				t.Errorf("%s: enum_titles of %q = %v, want a title for every value of %v", tpl.Code, name, got, f.Enum)
			}
		}

		if len(tpl.Items) == 0 {
			t.Errorf("%s: no agenda", tpl.Code)
		}
		for i, item := range tpl.Items {
			if item.Position != i+1 || !codes[item.DecisionCode] || item.Text == "" {
				t.Errorf("%s: agenda item %+v", tpl.Code, item)
			}
		}
	}
}
