package relay

import (
	"bytes"
	"encoding/json"
)

// Object is a JSON object that keeps its members' order and raw values, so a relayed message
// changes only where agx rewrites it.
type Object []Member

type Member struct {
	Key   string
	Value json.RawMessage
}

// Parse decodes raw as an object; ok is false for anything else, null included.
func Parse(raw json.RawMessage) (Object, bool) {
	d := json.NewDecoder(bytes.NewReader(raw))
	if t, err := d.Token(); err != nil || t != json.Delim('{') {
		return nil, false
	}
	o := Object{}
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
		o = append(o, Member{Key: key, Value: value})
	}
	if t, err := d.Token(); err != nil || t != json.Delim('}') {
		return nil, false
	}
	if _, err := d.Token(); err == nil {
		return nil, false
	}
	return o, true
}

// Get returns the value of key; JSON keeps the last of repeated keys.
func (o Object) Get(key string) (json.RawMessage, bool) {
	for i := len(o) - 1; i >= 0; i-- {
		if o[i].Key == key {
			return o[i].Value, true
		}
	}
	return nil, false
}

// Object returns the member key as an object.
func (o Object) Object(key string) (Object, bool) {
	raw, ok := o.Get(key)
	if !ok {
		return nil, false
	}
	return Parse(raw)
}

// Path follows nested objects by key.
func (o Object) Path(keys ...string) (Object, bool) {
	for _, key := range keys {
		var ok bool
		if o, ok = o.Object(key); !ok {
			return nil, false
		}
	}
	return o, true
}

// String returns the member key when it is a string.
func (o Object) String(key string) (string, bool) {
	raw, ok := o.Get(key)
	if !ok || len(bytes.TrimSpace(raw)) == 0 || bytes.TrimSpace(raw)[0] != '"' {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	return s, true
}

// Array returns the member key's elements when it is an array.
func (o Object) Array(key string) ([]json.RawMessage, bool) {
	raw, ok := o.Get(key)
	if !ok {
		return nil, false
	}
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil || values == nil {
		return nil, false
	}
	return values, true
}

// Set replaces the last member key in place, or appends it.
func (o *Object) Set(key string, value json.RawMessage) {
	for i := len(*o) - 1; i >= 0; i-- {
		if (*o)[i].Key == key {
			(*o)[i].Value = value
			return
		}
	}
	*o = append(*o, Member{Key: key, Value: value})
}

func (o Object) Marshal() json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(Encode(m.Key))
		b.WriteByte(':')
		b.Write(m.Value)
	}
	b.WriteByte('}')
	return b.Bytes()
}

// Encode marshals a value decoded from JSON or built from strings, which cannot fail; HTML
// characters stay as they are.
func Encode(v any) json.RawMessage {
	var b bytes.Buffer
	e := json.NewEncoder(&b)
	e.SetEscapeHTML(false)
	if err := e.Encode(v); err != nil {
		panic(err)
	}
	return bytes.TrimSuffix(b.Bytes(), []byte("\n"))
}

// EncodeArray joins raw values into a JSON array without re-encoding them.
func EncodeArray(values []json.RawMessage) json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('[')
	for i, v := range values {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(v)
	}
	b.WriteByte(']')
	return b.Bytes()
}
