package i18n

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"strings"
	"text/template"
	"text/template/parse"

	goi18n "github.com/nicksnyder/go-i18n/v2/i18n"
)

// JSON's normal map decoder silently overwrites duplicate IDs or plural forms.
func decodeObject(data []byte) (map[string]json.RawMessage, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	if token != json.Delim('{') {
		return nil, fmt.Errorf("expected JSON object")
	}
	fields := make(map[string]json.RawMessage)
	for d.More() {
		token, err := d.Token()
		if err != nil {
			return nil, err
		}
		key := token.(string)
		if _, exists := fields[key]; exists {
			return nil, fmt.Errorf("duplicate key %q", key)
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return nil, err
		}
		fields[key] = value
	}
	if _, err := d.Token(); err != nil {
		return nil, err
	}
	if _, err := d.Token(); err != io.EOF {
		return nil, fmt.Errorf("unexpected content after JSON object")
	}
	return fields, nil
}

func validateMessageFile(data []byte) (map[string]*goi18n.Message, error) {
	raw, err := decodeObject(data)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty catalog")
	}
	entries := make(map[string]*goi18n.Message, len(raw))
	for key, value := range raw {
		if key == "" || strings.TrimSpace(key) != key {
			return nil, fmt.Errorf("invalid message ID %q", key)
		}
		if _, err := decodeObject(value); err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		var message goi18n.Message
		d := json.NewDecoder(bytes.NewReader(value))
		d.DisallowUnknownFields()
		if err := d.Decode(&message); err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		if message.ID != "" && message.ID != key {
			return nil, fmt.Errorf("%s: conflicting message ID %q", key, message.ID)
		}
		message.ID = key
		if strings.TrimSpace(message.Other) == "" {
			return nil, fmt.Errorf("%s: other must not be empty", key)
		}
		if message.LeftDelim != "" || message.RightDelim != "" {
			return nil, fmt.Errorf("%s: use default template delimiters", key)
		}
		// This project uses plain named fields, with the same parameters in every plural form.
		fields, err := templateFields(message.Other)
		if err != nil {
			return nil, fmt.Errorf("%s.other: %w", key, err)
		}
		for form, text := range map[string]string{"zero": message.Zero, "one": message.One, "two": message.Two, "few": message.Few, "many": message.Many} {
			if text == "" {
				continue
			}
			otherFields, err := templateFields(text)
			if err != nil {
				return nil, fmt.Errorf("%s.%s: %w", key, form, err)
			}
			if !maps.Equal(fields, otherFields) {
				return nil, fmt.Errorf("%s.%s: template parameters differ from other", key, form)
			}
		}
		entries[key] = &message
	}
	return entries, nil
}

func validateCatalogParity(zh, en map[string]*goi18n.Message) error {
	for key, source := range zh {
		target, ok := en[key]
		if !ok {
			return fmt.Errorf("i18n: en-US missing key %q", key)
		}
		sourceFields, _ := templateFields(source.Other)
		targetFields, _ := templateFields(target.Other)
		if !maps.Equal(sourceFields, targetFields) {
			return fmt.Errorf("i18n: %s parameters differ between zh-CN and en-US", key)
		}
	}
	for key := range en {
		if _, ok := zh[key]; !ok {
			return fmt.Errorf("i18n: zh-CN missing key %q", key)
		}
	}
	return nil
}

// Restrict translation templates to text and {{.Name}} placeholders. Plural
// selection belongs to go-i18n/CLDR, not conditional logic in translation files.
func templateFields(text string) (map[string]bool, error) {
	t, err := template.New("message").Parse(text)
	if err != nil {
		return nil, err
	}
	fields := make(map[string]bool)
	for _, node := range t.Tree.Root.Nodes {
		switch n := node.(type) {
		case *parse.TextNode:
		case *parse.ActionNode:
			if len(n.Pipe.Decl) != 0 || len(n.Pipe.Cmds) != 1 || len(n.Pipe.Cmds[0].Args) != 1 {
				return nil, fmt.Errorf("use plain named parameters")
			}
			field, ok := n.Pipe.Cmds[0].Args[0].(*parse.FieldNode)
			if !ok || len(field.Ident) != 1 {
				return nil, fmt.Errorf("use plain named parameters")
			}
			fields[field.Ident[0]] = true
		default:
			return nil, fmt.Errorf("use plain named parameters")
		}
	}
	return fields, nil
}
