package topicplanning

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

// Contract §2.1, §3.1, §3.3; FR-001, FR-002, FR-006, FR-008, FR-011 - FR-013.

func validNodeBody() map[string]string {
	return map[string]string{
		"name":                `"双十一"`,
		"kind":                `"marketing"`,
		"starts_on":           `"2026-11-11"`,
		"ends_on":             `"2026-11-11"`,
		"timezone":            `"Asia/Shanghai"`,
		"lead_days":           `14`,
		"accounts":            `[{"account_id":"a1","role":"主推"}]`,
		"goal":                `"清库存 + 拉新"`,
		"material_source_ids": `["s1","s2"]`,
		"date_certainty":      `"confirmed"`,
		"date_basis":          `""`,
		"note":                `""`,
	}
}

func nodeJSON(fields map[string]string) []byte {
	parts := make([]string, 0, len(fields))
	for key, value := range fields {
		parts = append(parts, fmt.Sprintf("%q:%s", key, value))
	}
	return []byte("{" + strings.Join(parts, ",") + "}")
}

func withField(key, value string) map[string]string {
	body := validNodeBody()
	if value == "" {
		delete(body, key)
	} else {
		body[key] = value
	}
	return body
}

// createShape runs the create request through decoding and the shape check,
// which is everything the server does before it opens a transaction.
func createShape(data []byte) (NodeContent, error) {
	req, err := DecodeCreateNode(data)
	if err != nil {
		return NodeContent{}, err
	}
	return NormalizeNodeContent(req.Content)
}

func wantField(t *testing.T, err error, field string) {
	t.Helper()
	var fieldErr FieldError
	if !errors.As(err, &fieldErr) || fieldErr.Field != field {
		t.Fatalf("error = %v, want a 400 naming %q", err, field)
	}
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("field error %v does not map to 400", err)
	}
}

func TestValidNodeBodyIsAccepted(t *testing.T) {
	content, err := createShape(nodeJSON(validNodeBody()))
	if err != nil {
		t.Fatal(err)
	}
	if content.Name != "双十一" || content.LeadDays == nil || *content.LeadDays != 14 ||
		len(content.Accounts) != 1 || len(content.MaterialSourceIDs) != 2 {
		t.Fatalf("content = %+v", content)
	}
}

func TestNodeFieldsAreRefusedByName(t *testing.T) {
	long := func(n int) string { return `"` + strings.Repeat("字", n) + `"` }
	manyAccounts := make([]string, 21)
	for i := range manyAccounts {
		manyAccounts[i] = fmt.Sprintf(`{"account_id":"a%d"}`, i)
	}
	manySources := make([]string, 51)
	for i := range manySources {
		manySources[i] = fmt.Sprintf(`"s%d"`, i)
	}
	cases := []struct {
		name, key, value, field string
	}{
		{"unknown kind", "kind", `"festival"`, "kind"},
		{"empty name", "name", `"   "`, "name"},
		{"name too long", "name", long(201), "name"},
		{"end before start", "ends_on", `"2026-11-10"`, "ends_on"},
		{"span over 366 days", "ends_on", `"2027-11-12"`, "ends_on"},
		{"impossible date", "starts_on", `"2026-02-30"`, "starts_on"},
		{"empty timezone", "timezone", `""`, "timezone"},
		{"Local timezone", "timezone", `"Local"`, "timezone"},
		{"unknown timezone", "timezone", `"Mars/Olympus"`, "timezone"},
		{"negative lead", "lead_days", `-1`, "lead_days"},
		{"lead over a year", "lead_days", `366`, "lead_days"},
		{"fractional lead", "lead_days", `1.5`, "lead_days"},
		{"lead as text", "lead_days", `"3"`, "lead_days"},
		{"21 accounts", "accounts", "[" + strings.Join(manyAccounts, ",") + "]", "accounts"},
		{"blank account id", "accounts", `[{"account_id":"  "}]`, "accounts"},
		{"51 materials", "material_source_ids", "[" + strings.Join(manySources, ",") + "]", "material_source_ids"},
		{"goal too long", "goal", long(2001), "goal"},
		{"unknown certainty", "date_certainty", `"maybe"`, "date_certainty"},
		{"missing certainty", "date_certainty", "", "date_certainty"},
		{"basis too long", "date_basis", long(1001), "date_basis"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := createShape(nodeJSON(withField(tc.key, tc.value)))
			wantField(t, err, tc.field)
		})
	}
}

func TestAYearLongNodeIsAllowed(t *testing.T) {
	// 2028 is a leap year: 366 days, both ends included.
	body := validNodeBody()
	body["starts_on"], body["ends_on"] = `"2028-01-01"`, `"2028-12-31"`
	if _, err := createShape(nodeJSON(body)); err != nil {
		t.Fatalf("366-day node refused: %v", err)
	}
}

func TestUnknownKeysAndTrailingValuesAreRefused(t *testing.T) {
	body := validNodeBody()
	body["origin"] = `"import"`
	if _, err := DecodeCreateNode(nodeJSON(body)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("client-supplied origin = %v, want 400", err)
	}
	if _, err := DecodeCreateNode(append(nodeJSON(validNodeBody()), []byte(` {}`)...)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("trailing JSON value = %v, want 400", err)
	}
	if _, err := DecodeTransition([]byte(`{"base_revision":1,"status":"active"}`)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("unknown transition key = %v, want 400", err)
	}
}

// Missing and null are "not set"; 0 is "no preparation needed" (FR-013).
func TestLeadDaysKeepsUnsetApartFromZero(t *testing.T) {
	missing, err := createShape(nodeJSON(withField("lead_days", "")))
	if err != nil || missing.LeadDays != nil {
		t.Fatalf("missing lead = %v, %v; want unset", missing.LeadDays, err)
	}
	null, err := createShape(nodeJSON(withField("lead_days", "null")))
	if err != nil || null.LeadDays != nil {
		t.Fatalf("null lead = %v, %v; want unset", null.LeadDays, err)
	}
	zero, err := createShape(nodeJSON(withField("lead_days", "0")))
	if err != nil || zero.LeadDays == nil || *zero.LeadDays != 0 {
		t.Fatalf("zero lead = %v, %v; want 0", zero.LeadDays, err)
	}
}

func TestAccountsAreDeduplicatedKeepingTheFirst(t *testing.T) {
	content, err := createShape(nodeJSON(withField("accounts",
		`[{"account_id":"a1","role":"主推"},{"account_id":" a1 ","role":"陪跑"},{"account_id":"a2"}]`)))
	if err != nil {
		t.Fatal(err)
	}
	want := []NodeAccount{{AccountID: "a1", Role: "主推"}, {AccountID: "a2"}}
	if len(content.Accounts) != 2 || content.Accounts[0] != want[0] || content.Accounts[1] != want[1] {
		t.Fatalf("accounts = %+v, want %+v", content.Accounts, want)
	}
	// Twenty distinct accounts after de-duplication is the limit, not twenty
	// entries before it.
	entries := make([]string, 0, 25)
	for i := range 20 {
		entries = append(entries, fmt.Sprintf(`{"account_id":"a%d"}`, i))
	}
	entries = append(entries, `{"account_id":"a0"}`, `{"account_id":"a1"}`)
	if _, err := createShape(nodeJSON(withField("accounts", "["+strings.Join(entries, ",")+"]"))); err != nil {
		t.Fatalf("20 distinct accounts with repeats refused: %v", err)
	}
}

func TestMaterialsUseTheSharedSourceNormalization(t *testing.T) {
	content, err := createShape(nodeJSON(withField("material_source_ids", `[" s1 ","","s1","s2"]`)))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(content.MaterialSourceIDs, ",") != "s1,s2" {
		t.Fatalf("materials = %v, want [s1 s2]", content.MaterialSourceIDs)
	}
	empty, err := createShape(nodeJSON(withField("material_source_ids", "")))
	if err != nil || empty.MaterialSourceIDs == nil || len(empty.MaterialSourceIDs) != 0 {
		t.Fatalf("missing materials = %#v, %v; want []", empty.MaterialSourceIDs, err)
	}
}

func TestReviseAndTransitionsNeedABaseRevision(t *testing.T) {
	if _, err := DecodeReviseNode(nodeJSON(validNodeBody())); err == nil {
		t.Fatal("revise without base_revision accepted")
	} else {
		wantField(t, err, "base_revision")
	}
	body := validNodeBody()
	body["base_revision"] = "2"
	req, err := DecodeReviseNode(nodeJSON(body))
	if err != nil || req.BaseRevision != 2 {
		t.Fatalf("revise = %+v, %v", req, err)
	}
	if _, err = DecodeTransition([]byte(`{"note":"x"}`)); err == nil {
		t.Fatal("transition without base_revision accepted")
	} else {
		wantField(t, err, "base_revision")
	}
}

func TestImportLimitsAndPerRowErrors(t *testing.T) {
	row := string(nodeJSON(func() map[string]string {
		b := validNodeBody()
		delete(b, "note")
		return b
	}()))
	rows := func(n int) []byte {
		items := make([]string, n)
		for i := range items {
			items[i] = row
		}
		return []byte(`{"rows":[` + strings.Join(items, ",") + `]}`)
	}
	if _, err := DecodeImport(rows(101)); err == nil {
		t.Fatal("101 import rows accepted")
	} else {
		wantField(t, err, "rows")
	}
	if _, err := DecodeImport(rows(0)); err == nil {
		t.Fatal("empty import accepted")
	}
	decoded, err := DecodeImport(rows(100))
	if err != nil || len(decoded) != 100 {
		t.Fatalf("100 rows = %d, %v", len(decoded), err)
	}

	// One bad row of each kind; the good one next to them still decodes.
	mixed := []byte(`{"rows":[` + row + `,{"name":"x","kind":"festival","starts_on":"2026-01-01","ends_on":"2026-01-01","timezone":"UTC","date_certainty":"confirmed"},{"name":"x","note":"no note in import"},42]}`)
	decoded, err = DecodeImport(mixed)
	if err != nil {
		t.Fatal(err)
	}
	if decoded[0].Err != nil {
		t.Fatalf("good row failed: %v", decoded[0].Err)
	}
	if invalidField(decoded[1].Err) != "kind" {
		t.Fatalf("bad kind row = %v", decoded[1].Err)
	}
	if invalidField(decoded[2].Err) != "row" || invalidField(decoded[3].Err) != "row" {
		t.Fatalf("malformed rows = %v / %v", decoded[2].Err, decoded[3].Err)
	}
}

func baseContent() NodeContent {
	return NodeContent{
		Name: "双十一", Kind: NodeKindMarketing, StartsOn: "2026-11-11", EndsOn: "2026-11-11",
		Timezone: "Asia/Shanghai", LeadDays: nil, Accounts: []NodeAccount{{AccountID: "a1", Role: "主推"}},
		Goal: "清库存", MaterialSourceIDs: []string{"s1"}, DateCertainty: DateConfirmed,
	}
}

func TestChangeKindIsDecidedFromWhatChanged(t *testing.T) {
	previous := baseContent()

	goal := baseContent()
	goal.Goal = "拉新"
	if kind, err := ClassifyChange(previous, goal); err != nil || kind != ChangeEdit {
		t.Fatalf("goal only = %s, %v; want edit", kind, err)
	}

	unsetToZero := baseContent()
	unsetToZero.LeadDays = lead(0)
	if kind, err := ClassifyChange(previous, unsetToZero); err != nil || kind != ChangeReschedule {
		t.Fatalf("lead unset -> 0 = %s, %v; want reschedule", kind, err)
	}

	zone := baseContent()
	zone.Timezone = "America/Los_Angeles"
	if kind, _ := ClassifyChange(previous, zone); kind != ChangeReschedule {
		t.Fatalf("timezone = %s, want reschedule", kind)
	}

	ends := baseContent()
	ends.EndsOn = "2026-11-12"
	if kind, _ := ClassifyChange(previous, ends); kind != ChangeReschedule {
		t.Fatalf("ends_on = %s, want reschedule", kind)
	}

	accounts := baseContent()
	accounts.Accounts = []NodeAccount{{AccountID: "a1", Role: "陪跑"}}
	if kind, _ := ClassifyChange(previous, accounts); kind != ChangeEdit {
		t.Fatalf("role = %s, want edit", kind)
	}

	_, err := ClassifyChange(previous, baseContent())
	wantField(t, err, "revision")
}

func TestStatusFilterIsTheControlledSet(t *testing.T) {
	if status, err := ParseNodeStatusFilter(""); err != nil || status != "" {
		t.Fatalf("empty filter = %q, %v", status, err)
	}
	if status, err := ParseNodeStatusFilter("unconfirmed"); err != nil || status != NodeStatusUnconfirmed {
		t.Fatalf("unconfirmed filter = %q, %v", status, err)
	}
	_, err := ParseNodeStatusFilter("deleted")
	wantField(t, err, "status")
}
