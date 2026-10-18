package appserver

import (
	"bytes"
	"encoding/json"
	"maps"
	"slices"
)

// object is a JSON object that keeps its members' order and raw values, so a relayed message
// changes only where agx rewrites it.
type object []member

type member struct {
	key   string
	value json.RawMessage
}

// parseObject decodes raw as an object; ok is false for anything else, null included.
func parseObject(raw json.RawMessage) (object, bool) {
	d := json.NewDecoder(bytes.NewReader(raw))
	if t, err := d.Token(); err != nil || t != json.Delim('{') {
		return nil, false
	}
	o := object{}
	for d.More() {
		t, err := d.Token()
		if err != nil {
			return nil, false
		}
		key, ok := t.(string)
		if !ok {
			return nil, false
		}
		var value json.RawMessage
		if err := d.Decode(&value); err != nil {
			return nil, false
		}
		o = append(o, member{key: key, value: value})
	}
	if t, err := d.Token(); err != nil || t != json.Delim('}') {
		return nil, false
	}
	if _, err := d.Token(); err == nil {
		return nil, false
	}
	return o, true
}

// get returns the value of key; JSON keeps the last of repeated keys.
func (o object) get(key string) (json.RawMessage, bool) {
	for i := len(o) - 1; i >= 0; i-- {
		if o[i].key == key {
			return o[i].value, true
		}
	}
	return nil, false
}

// object returns the member key as an object.
func (o object) object(key string) (object, bool) {
	raw, ok := o.get(key)
	if !ok {
		return nil, false
	}
	return parseObject(raw)
}

// string returns the member key when it is a string.
func (o object) string(key string) (string, bool) {
	raw, ok := o.get(key)
	if !ok || len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '"' {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

// set replaces the last member key in place, or appends it.
func (o *object) set(key string, value json.RawMessage) {
	for i := len(*o) - 1; i >= 0; i-- {
		if (*o)[i].key == key {
			(*o)[i].value = value
			return
		}
	}
	*o = append(*o, member{key: key, value: value})
}

func (o object) marshal() json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(encode(m.key))
		b.WriteByte(':')
		b.Write(m.value)
	}
	b.WriteByte('}')
	return b.Bytes()
}

// encode marshals a value decoded from JSON or built from strings, which cannot fail; HTML
// characters stay as they are.
func encode(v any) json.RawMessage {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		panic(err)
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

// merge adds ours to theirs without changing a value theirs sets: shared keys whose values are
// both objects merge recursively, and theirs wins any other conflict. New keys come first, so a
// dotted key of theirs (Codex applies "mcp_servers.x" after the keys before it) still applies.
func merge(theirs object, ours map[string]any) object {
	added := object{}
	for _, key := range slices.Sorted(maps.Keys(ours)) {
		value := ours[key]
		existing, ok := theirs.get(key)
		if !ok {
			added = append(added, member{key: key, value: encode(value)})
			continue
		}
		nested, isMap := value.(map[string]any)
		sub, isObject := parseObject(existing)
		if isMap && isObject {
			theirs.set(key, merge(sub, nested).marshal())
		}
	}
	return append(added, theirs...)
}
