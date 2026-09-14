package diagnostics

import (
	"reflect"
	"testing"
)

// The input snapshot records versions, references and hashes. It carries no
// image, audio or attachment bytes, and D13-V06 says it must stay that way.
//
// Today that holds only because nobody has added such a field. Asserting "no
// []byte field exists" would not protect it either: a string field holding a
// base64 image passes that check. So the assertion is the field list itself.
// Adding ANY field fails this test until a person confirms it is not media.
// That cost is the point - see specs/008 Edge Cases.
var expectedSnapshotFields = []struct {
	name string
	kind reflect.Kind
}{
	{"ConfigVersion", reflect.String},
	{"PersonaRef", reflect.String},
	{"SOPVersion", reflect.String},
	{"SkillVersion", reflect.String},
	{"RuleVersion", reflect.String},
	{"Executor", reflect.String},
	{"ExecutorVersion", reflect.String},
	{"Scope", reflect.String},
	{"Preference", reflect.String},
	{"Required", reflect.Slice},
	{"Excluded", reflect.Slice},
	{"Grants", reflect.Slice},
	{"Hashes", reflect.Map},
	{"Temperature", reflect.Float64},
	{"Budget", reflect.Int},
	{"Timeout", reflect.Int},
}

func TestSnapshotHasNoMediaPayload(t *testing.T) {
	actual := reflect.TypeOf(Snapshot{})
	if actual.NumField() != len(expectedSnapshotFields) {
		var got []string
		for i := 0; i < actual.NumField(); i++ {
			got = append(got, actual.Field(i).Name)
		}
		t.Fatalf("Snapshot has %d fields, the reviewed list has %d.\ngot:  %v\nIf a field was added, confirm it carries no image, audio or attachment "+
			"content and then add it to expectedSnapshotFields.",
			actual.NumField(), len(expectedSnapshotFields), got)
	}
	for i, want := range expectedSnapshotFields {
		field := actual.Field(i)
		if field.Name != want.name {
			t.Errorf("field %d: got %q, want %q", i, field.Name, want.name)
			continue
		}
		if field.Type.Kind() != want.kind {
			t.Errorf("field %s: kind %s, want %s - a changed shape can change what "+
				"the field is able to carry", field.Name, field.Type.Kind(), want.kind)
		}
	}
}

// The element types matter as much as the outer shape: []string cannot hold a
// blob, [][]byte can. Checking only reflect.Slice above would miss that.
func TestSnapshotCollectionsCarryOnlyText(t *testing.T) {
	actual := reflect.TypeOf(Snapshot{})
	for _, name := range []string{"Required", "Excluded", "Grants"} {
		field, ok := actual.FieldByName(name)
		if !ok {
			t.Fatalf("field %s is gone", name)
		}
		if field.Type.Elem().Kind() != reflect.String {
			t.Errorf("%s holds %s, want string", name, field.Type.Elem().Kind())
		}
	}
	hashes, ok := actual.FieldByName("Hashes")
	if !ok {
		t.Fatal("field Hashes is gone")
	}
	if hashes.Type.Key().Kind() != reflect.String || hashes.Type.Elem().Kind() != reflect.String {
		t.Errorf("Hashes is map[%s]%s, want map[string]string",
			hashes.Type.Key().Kind(), hashes.Type.Elem().Kind())
	}
}

// No field anywhere in the snapshot may be a byte container, whatever it is
// called. This is the cheap check; the field list above is the real one.
func TestSnapshotHasNoByteContainerAnywhere(t *testing.T) {
	actual := reflect.TypeOf(Snapshot{})
	for i := 0; i < actual.NumField(); i++ {
		field := actual.Field(i)
		if field.Type.Kind() == reflect.Slice && field.Type.Elem().Kind() == reflect.Uint8 {
			t.Errorf("field %s is a byte slice: the snapshot must not carry raw bytes", field.Name)
		}
	}
}
