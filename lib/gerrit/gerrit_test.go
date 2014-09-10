package gerrit

import (
	"reflect"
	"strings"
	"testing"
)

func TestParseQueryResults(t *testing.T) {
	input := `)]}'
	[
		{
			"change_id": "I26f771cebd6e512b89e98bec1fadfa1cb2aad6e8",
			"current_revision": "3654e38b2f80a5410ea94f1d7321477d89cac391",
			"project": "veyron",
			"revisions": {
				"3654e38b2f80a5410ea94f1d7321477d89cac391": {
					"fetch": {
						"http": {
							"ref": "refs/changes/40/4440/1"
						}
					}
				}
			}
		},
		{
			"change_id": "I35d83f8adae5b7db1974062fdc744f700e456677",
			"current_revision": "b60413712472f1b576c7be951c4de309c6edaa53",
			"project": "tools",
			"revisions": {
				"b60413712472f1b576c7be951c4de309c6edaa53": {
					"fetch": {
						"http": {
							"ref": "refs/changes/43/4443/1"
						}
					}
				}
			}
		}
	]
	`

	expected := []QueryResult{
		{
			Ref:      "refs/changes/40/4440/1",
			Repo:     "veyron",
			ChangeID: "I26f771cebd6e512b89e98bec1fadfa1cb2aad6e8",
		},
		{
			Ref:      "refs/changes/43/4443/1",
			Repo:     "tools",
			ChangeID: "I35d83f8adae5b7db1974062fdc744f700e456677",
		},
	}

	got, err := parseQueryResults(strings.NewReader(input))
	if err != nil {
		t.Fatalf("%v", err)
	}
	if !reflect.DeepEqual(expected, got) {
		t.Errorf("want: %#v, got: %#v", expected, got)
	}
}
