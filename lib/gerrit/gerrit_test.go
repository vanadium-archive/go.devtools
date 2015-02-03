package gerrit

import (
	"fmt"
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
			"project": "vanadium",
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
			"change_id": "I26f771cebd6e512b89e98bec1fadfa1cb2aad6e8",
			"current_revision": "3654e38b2f80a5410ea94f1d7321477d89cac391",
			"labels": {
				"Code-Review": {},
				"Verified": {}
			},
			"project": "vanadium",
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
					},
					"commit": {
						"message": "this change is great.\nPresubmitTest: none"
					}
				}
			}
		}
	]
	`

	expected := []QueryResult{
		{
			ChangeID:      "I26f771cebd6e512b89e98bec1fadfa1cb2aad6e8",
			PresubmitTest: PresubmitTestTypeAll,
			Ref:           "refs/changes/40/4440/1",
			Project:       "vanadium",
		},
		{
			ChangeID:      "I26f771cebd6e512b89e98bec1fadfa1cb2aad6e8",
			PresubmitTest: PresubmitTestTypeAll,
			Labels: map[string]struct{}{
				"Code-Review": struct{}{},
				"Verified":    struct{}{},
			},
			Ref:     "refs/changes/40/4440/1",
			Project: "vanadium",
		},
		{
			ChangeID:      "I35d83f8adae5b7db1974062fdc744f700e456677",
			PresubmitTest: PresubmitTestTypeNone,
			Ref:           "refs/changes/43/4443/1",
			Project:       "tools",
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

func TestParseMultiPartMatch(t *testing.T) {
	type testCase struct {
		str             string
		expectNoMatches bool
		expectedIndex   string
		expectedTotal   string
	}
	testCases := []testCase{
		testCase{
			str:             "message...\nMultiPart: a/3",
			expectNoMatches: true,
		},
		testCase{
			str:             "message...\n1/3",
			expectNoMatches: true,
		},
		testCase{
			str:           "message...\nMultiPart:1/2",
			expectedIndex: "1",
			expectedTotal: "2",
		},
		testCase{
			str:           "message...\nMultiPart: 1/2",
			expectedIndex: "1",
			expectedTotal: "2",
		},
		testCase{
			str:           "message...\nMultiPart: 1 /2",
			expectedIndex: "1",
			expectedTotal: "2",
		},
		testCase{
			str:           "message...\nMultiPart: 1/ 2",
			expectedIndex: "1",
			expectedTotal: "2",
		},
		testCase{
			str:           "message...\nMultiPart: 1 / 2",
			expectedIndex: "1",
			expectedTotal: "2",
		},
		testCase{
			str:           "message...\nMultiPart: 123/234",
			expectedIndex: "123",
			expectedTotal: "234",
		},
	}
	for _, test := range testCases {
		multiPartCLInfo, _ := parseMultiPartMatch(test.str)
		if test.expectNoMatches && multiPartCLInfo != nil {
			t.Fatalf("want no matches, got %v", multiPartCLInfo)
		}
		if !test.expectNoMatches && multiPartCLInfo == nil {
			t.Fatalf("want matches, got no matches")
		}
		if !test.expectNoMatches {
			if want, got := test.expectedIndex, fmt.Sprintf("%d", multiPartCLInfo.Index); want != got {
				t.Fatalf("want 'index' %q, got %q", want, got)
			}
			if want, got := test.expectedTotal, fmt.Sprintf("%d", multiPartCLInfo.Total); want != got {
				t.Fatalf("want 'total' %q, got %q", want, got)
			}
		}
	}
}
