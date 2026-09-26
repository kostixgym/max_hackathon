package rules

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"regexp"
	"slices"
	"strconv"
	"unicode/utf8"
)

// Form parameters of a template (docs/04, решение 14): the template keeps the schema of
// the form, the initiative keeps the values. The schema is JSON Schema Draft 2020-12
// limited to what the mini-app renders (docs/API_DESCRIPTION.md, GET /templates/{code}):
// an object of flat fields of type string, integer, number or boolean with enum,
// required, minimum, maximum, minLength and maxLength. A schema outside this subset
// fails when the catalog is seeded, so the server never accepts a form the mini-app
// cannot show.

// ErrInvalidParams means the values of the form do not match the template schema.
var ErrInvalidParams = errors.New("initiative params do not match the template schema")

// Reasons of a ParamsError: machine codes, the API turns them into text.
const (
	ParamsNotObject    = "not_object"
	ParamsUnknownField = "unknown_field"
	ParamsRequired     = "required"
	ParamsType         = "type"
	ParamsEnum         = "enum"
	ParamsMinimum      = "minimum"
	ParamsMaximum      = "maximum"
	ParamsMinLength    = "min_length"
	ParamsMaxLength    = "max_length"
)

// ParamsError names the field that failed and why. It matches ErrInvalidParams.
type ParamsError struct {
	Field  string // the key in params; empty when params is not an object
	Title  string // the label of the field in the form
	Reason string
	Limit  string // the bound that failed: minimum, maximum or a length
}

func (e *ParamsError) Error() string {
	if e.Field == "" {
		return "params: " + e.Reason
	}

	return fmt.Sprintf("params.%s: %s", e.Field, e.Reason)
}

func (e *ParamsError) Unwrap() error { return ErrInvalidParams }

// jsonSchemaDraft is the only dialect a template schema may declare.
const jsonSchemaDraft = "https://json-schema.org/draft/2020-12/schema"

// fieldName: keys of params are identifiers of the mini-app code.
var fieldName = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)

// ParamsSchema is a compiled form schema.
type ParamsSchema struct {
	fields   map[string]paramField
	names    []string // sorted: the first error is the same on every call
	required []string
}

// paramField is one property of the schema as it is written in JSON.
type paramField struct {
	Type        string       `json:"type"`
	Title       string       `json:"title"`
	Description string       `json:"description"`
	Enum        []string     `json:"enum"`
	Minimum     *json.Number `json:"minimum"`
	Maximum     *json.Number `json:"maximum"`
	MinLength   *int         `json:"minLength"`
	MaxLength   *int         `json:"maxLength"`
}

// CompileParamsSchema parses a template schema and checks that it stays in the subset.
func CompileParamsSchema(raw []byte) (*ParamsSchema, error) {
	var doc struct {
		Schema               string                `json:"$schema"`
		Type                 string                `json:"type"`
		AdditionalProperties *bool                 `json:"additionalProperties"`
		Required             []string              `json:"required"`
		Properties           map[string]paramField `json:"properties"`
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields() // a keyword outside the subset is an error, not ignored
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("params schema: %w", err)
	}
	if err := expectEOF(dec); err != nil {
		return nil, fmt.Errorf("params schema: %w", err)
	}

	switch {
	case doc.Schema != "" && doc.Schema != jsonSchemaDraft:
		return nil, fmt.Errorf("params schema: $schema %q, want %q", doc.Schema, jsonSchemaDraft)
	case doc.Type != "object":
		return nil, fmt.Errorf("params schema: type %q, want object", doc.Type)
	case doc.AdditionalProperties == nil || *doc.AdditionalProperties:
		// The mini-app shows only the declared fields: others would be stored unseen.
		return nil, errors.New("params schema: additionalProperties must be false")
	}

	s := &ParamsSchema{fields: doc.Properties}
	for name, f := range doc.Properties {
		if err := f.check(name); err != nil {
			return nil, fmt.Errorf("params schema: %w", err)
		}
		s.names = append(s.names, name)
	}
	slices.Sort(s.names)

	for _, name := range doc.Required {
		if _, ok := doc.Properties[name]; !ok {
			return nil, fmt.Errorf("params schema: required field %q is not a property", name)
		}
		if slices.Contains(s.required, name) {
			return nil, fmt.Errorf("params schema: required field %q is listed twice", name)
		}
		s.required = append(s.required, name)
	}

	return s, nil
}

// check validates the definition of one field.
func (f paramField) check(name string) error {
	if !fieldName.MatchString(name) {
		return fmt.Errorf("field %q: the name must be snake_case", name)
	}
	if f.Title == "" {
		return fmt.Errorf("field %q: title is required, the form shows it", name)
	}

	switch f.Type {
	case "string":
		if f.Minimum != nil || f.Maximum != nil {
			return fmt.Errorf("field %q: minimum and maximum are for numbers", name)
		}
		if f.Enum == nil && f.MaxLength == nil {
			// Free text must be bounded: otherwise one field takes the whole request body.
			return fmt.Errorf("field %q: a string needs maxLength or enum", name)
		}
		if f.Enum != nil && (f.MinLength != nil || f.MaxLength != nil) {
			return fmt.Errorf("field %q: an enum takes no length limits", name)
		}
	case "integer", "number":
		if f.Enum != nil || f.MinLength != nil || f.MaxLength != nil {
			return fmt.Errorf("field %q: enum and lengths are for strings", name)
		}
	case "boolean":
		if f.Enum != nil || f.Minimum != nil || f.Maximum != nil || f.MinLength != nil || f.MaxLength != nil {
			return fmt.Errorf("field %q: a boolean takes no limits", name)
		}
	default:
		return fmt.Errorf("field %q: type %q is not supported", name, f.Type)
	}

	if f.Enum != nil {
		if len(f.Enum) == 0 {
			return fmt.Errorf("field %q: enum is empty", name)
		}
		for i, v := range f.Enum {
			if v == "" || slices.Contains(f.Enum[:i], v) {
				return fmt.Errorf("field %q: enum values must be unique and not empty", name)
			}
		}
	}

	var lo, hi float64
	var err error
	if f.Minimum != nil {
		if lo, err = finite(*f.Minimum); err != nil {
			return fmt.Errorf("field %q: minimum: %w", name, err)
		}
	}
	if f.Maximum != nil {
		if hi, err = finite(*f.Maximum); err != nil {
			return fmt.Errorf("field %q: maximum: %w", name, err)
		}
	}
	if f.Minimum != nil && f.Maximum != nil && lo > hi {
		return fmt.Errorf("field %q: minimum is above maximum", name)
	}
	if (f.MinLength != nil && *f.MinLength < 0) || (f.MaxLength != nil && *f.MaxLength < 0) ||
		(f.MinLength != nil && f.MaxLength != nil && *f.MinLength > *f.MaxLength) {
		return fmt.Errorf("field %q: wrong length limits", name)
	}

	return nil
}

// Validate checks the values of the form. Empty params (nil or null) are an empty
// object: a form without required fields may be sent without values.
func (s *ParamsSchema) Validate(params []byte) error {
	values := map[string]any{}
	if trimmed := bytes.TrimSpace(params); len(trimmed) > 0 && !bytes.Equal(trimmed, []byte("null")) {
		// Numbers stay text: the bounds are checked on the number as it was written.
		dec := json.NewDecoder(bytes.NewReader(trimmed))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil || expectEOF(dec) != nil {
			return &ParamsError{Reason: ParamsNotObject}
		}
		obj, ok := v.(map[string]any)
		if !ok {
			return &ParamsError{Reason: ParamsNotObject}
		}
		values = obj
	}

	unknown := make([]string, 0)
	for name := range values {
		if _, ok := s.fields[name]; !ok {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		slices.Sort(unknown)

		return &ParamsError{Field: unknown[0], Reason: ParamsUnknownField}
	}

	for _, name := range s.required {
		if _, ok := values[name]; !ok {
			return &ParamsError{Field: name, Title: s.fields[name].Title, Reason: ParamsRequired}
		}
	}

	for _, name := range s.names {
		if v, ok := values[name]; ok {
			if err := s.fields[name].validate(name, v); err != nil {
				return err
			}
		}
	}

	return nil
}

// validate checks one value against its field. A value is never null: an optional
// field without a value is left out of params.
func (f paramField) validate(name string, v any) error {
	fail := func(reason, limit string) error {
		return &ParamsError{Field: name, Title: f.Title, Reason: reason, Limit: limit}
	}

	switch f.Type {
	case "string":
		s, ok := v.(string)
		if !ok {
			return fail(ParamsType, "")
		}
		if f.Enum != nil && !slices.Contains(f.Enum, s) {
			return fail(ParamsEnum, "")
		}
		// JSON Schema counts characters, not bytes.
		n := utf8.RuneCountInString(s)
		if f.MinLength != nil && n < *f.MinLength {
			return fail(ParamsMinLength, strconv.Itoa(*f.MinLength))
		}
		if f.MaxLength != nil && n > *f.MaxLength {
			return fail(ParamsMaxLength, strconv.Itoa(*f.MaxLength))
		}
	case "integer", "number":
		num, ok := v.(json.Number)
		if !ok {
			return fail(ParamsType, "")
		}
		// float64 is exact for the bounds of the form (counts and sums in rubles). The
		// value is not parsed as an exact rational on purpose: «1e999999» would cost
		// megabytes to expand.
		x, err := finite(num)
		if err != nil || (f.Type == "integer" && x != math.Trunc(x)) {
			return fail(ParamsType, "")
		}
		if f.Minimum != nil {
			if lo, _ := finite(*f.Minimum); x < lo {
				return fail(ParamsMinimum, f.Minimum.String())
			}
		}
		if f.Maximum != nil {
			if hi, _ := finite(*f.Maximum); x > hi {
				return fail(ParamsMaximum, f.Maximum.String())
			}
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			return fail(ParamsType, "")
		}
	}

	return nil
}

// finite parses a JSON number that fits a float64.
func finite(n json.Number) (float64, error) {
	x, err := strconv.ParseFloat(n.String(), 64)
	if err != nil {
		return 0, err
	}
	if math.IsInf(x, 0) || math.IsNaN(x) {
		return 0, fmt.Errorf("%s is out of range", n)
	}

	return x, nil
}

// expectEOF fails when the decoder has more than one JSON value.
func expectEOF(dec *json.Decoder) error {
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("unexpected data after the JSON value")
	}

	return nil
}
