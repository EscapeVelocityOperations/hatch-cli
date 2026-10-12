package mcpserver

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/EscapeVelocityOperations/hatch-cli/internal/api"
)

// --- set_email_protection ---

func TestSetEmailProtectionHandler_MissingParams(t *testing.T) {
	result, err := setEmailProtectionHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestSetEmailProtectionHandler_NoListsIsValidationError(t *testing.T) {
	result, err := setEmailProtectionHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "at least one")
}

func TestSetEmailProtectionHandler_Unauthenticated(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := setEmailProtectionHandler(context.Background(), makeReq(map[string]interface{}{
		"app":    "myapp-a1b2",
		"emails": []interface{}{"a@b.com"},
	}))
	assertError(t, result, err, "not authenticated")
}

func TestSetEmailProtectionHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"POST /v1/apps/myapp-a1b2/email-protect": jsonHandler(api.EmailProtection{
			EmailProtected: true,
			Emails:         []string{"a@b.com"},
			Domains:        []string{"corp.com"},
		}),
	})

	result, err := setEmailProtectionHandler(context.Background(), makeReq(map[string]interface{}{
		"app":     "myapp-a1b2",
		"emails":  []interface{}{"a@b.com"},
		"domains": []interface{}{"corp.com"},
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "a@b.com") || !strings.Contains(text, "corp.com") {
		t.Errorf("expected emails/domains in output, got: %s", text)
	}
}

// --- get_email_protection ---

func TestGetEmailProtectionHandler_MissingParams(t *testing.T) {
	result, err := getEmailProtectionHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestGetEmailProtectionHandler_Unauthenticated(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := getEmailProtectionHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "not authenticated")
}

func TestGetEmailProtectionHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/myapp-a1b2/email-protect": jsonHandler(api.EmailProtection{
			EmailProtected: true,
			Emails:         []string{"a@b.com"},
		}),
	})

	result, err := getEmailProtectionHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "a@b.com") {
		t.Errorf("expected email in output, got: %s", text)
	}
}

// TestGetEmailProtectionHandler_MailerConfigured is a regression guard
// (mirrors the h-ppn8 MCP registration guard style): the tool marshals the
// api.EmailProtection struct directly, so mailer_configured must round-trip
// into the result JSON without any handler-side field list to drift out of
// sync (h-7b9l T-003).
func TestGetEmailProtectionHandler_MailerConfigured(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/myapp-a1b2/email-protect": jsonHandler(api.EmailProtection{
			EmailProtected:   true,
			Emails:           []string{"a@b.com"},
			MailerConfigured: false,
		}),
	})

	result, err := getEmailProtectionHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, `"mailer_configured":false`) {
		t.Errorf("expected mailer_configured in result JSON, got: %s", text)
	}
}

// --- disable_email_protection ---

func TestDisableEmailProtectionHandler_MissingParams(t *testing.T) {
	result, err := disableEmailProtectionHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestDisableEmailProtectionHandler_Unauthenticated(t *testing.T) {
	saveAndRestore(t)
	setNoAuth()

	result, err := disableEmailProtectionHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "not authenticated")
}

func TestDisableEmailProtectionHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"DELETE /v1/apps/myapp-a1b2/email-protect": jsonHandler(api.EmailProtection{EmailProtected: false}),
	})

	result, err := disableEmailProtectionHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "myapp-a1b2") {
		t.Errorf("expected app slug in output, got: %s", text)
	}
}

// --- add_email_protection_user (h-7b9l/h-dmd4 T-003, formula tests-first —
// NOT implemented yet: addEmailProtectionUserHandler/Tool do not exist on
// this branch. RED via undefined identifier is the expected/correct state
// until the impl-cli formula step lands T-004.) ---

func TestAddEmailProtectionUserHandler_MissingParams(t *testing.T) {
	result, err := addEmailProtectionUserHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestAddEmailProtectionUserHandler_NoListsIsValidationError(t *testing.T) {
	result, err := addEmailProtectionUserHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "at least one")
}

// TestAddEmailProtectionUserHandler_MergesWithExisting is the core D1/T-003
// assertion: GET the current lists, normalize the new entry, merge (never
// clobber), POST the union — and the result reports the post-write state so
// an agent can verify its own write (D4).
func TestAddEmailProtectionUserHandler_MergesWithExisting(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")

	var gotBody map[string]any
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/myapp-a1b2/email-protect": jsonHandler(api.EmailProtection{
			EmailProtected: true,
			Emails:         []string{"a@b.com"},
			Domains:        []string{"b.com"},
		}),
		"POST /v1/apps/myapp-a1b2/email-protect": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			jsonHandler(api.EmailProtection{
				EmailProtected: true,
				Emails:         []string{"a@b.com", "c@d.com"},
				Domains:        []string{"b.com"},
			})(w, r)
		},
	})

	result, err := addEmailProtectionUserHandler(context.Background(), makeReq(map[string]interface{}{
		"app":    "myapp-a1b2",
		"emails": []interface{}{"C@D.com "},
	}))
	text := assertSuccess(t, result, err)

	emails, _ := gotBody["emails"].([]any)
	if len(emails) != 2 || emails[0] != "a@b.com" || emails[1] != "c@d.com" {
		t.Errorf("POST body emails = %v, want existing+normalized-new merged [a@b.com c@d.com]", gotBody["emails"])
	}
	domains, _ := gotBody["domains"].([]any)
	if len(domains) != 1 || domains[0] != "b.com" {
		t.Errorf("POST body domains = %v, want existing [b.com] preserved (not clobbered)", gotBody["domains"])
	}
	if !strings.Contains(text, "c@d.com") {
		t.Errorf("expected post-write state (c@d.com) in result, got: %s", text)
	}
}

// --- remove_email_protection_user (h-7b9l/h-dmd4 T-005, formula tests-first) ---

func TestRemoveEmailProtectionUserHandler_MissingParams(t *testing.T) {
	result, err := removeEmailProtectionUserHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestRemoveEmailProtectionUserHandler_NoListsIsValidationError(t *testing.T) {
	result, err := removeEmailProtectionUserHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	assertError(t, result, err, "at least one")
}

func TestRemoveEmailProtectionUserHandler_FiltersExisting(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")

	var gotBody map[string]any
	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/myapp-a1b2/email-protect": jsonHandler(api.EmailProtection{
			EmailProtected: true,
			Emails:         []string{"a@b.com", "c@d.com"},
		}),
		"POST /v1/apps/myapp-a1b2/email-protect": func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewDecoder(r.Body).Decode(&gotBody)
			jsonHandler(api.EmailProtection{EmailProtected: true, Emails: []string{"a@b.com"}})(w, r)
		},
	})

	result, err := removeEmailProtectionUserHandler(context.Background(), makeReq(map[string]interface{}{
		"app":    "myapp-a1b2",
		"emails": []interface{}{"c@d.com"},
	}))
	assertSuccess(t, result, err)

	emails, _ := gotBody["emails"].([]any)
	if len(emails) != 1 || emails[0] != "a@b.com" {
		t.Errorf("POST body emails = %v, want [a@b.com] (c@d.com removed)", gotBody["emails"])
	}
}

// TestRemoveEmailProtectionUserHandler_RemovingLastEntryWarns (D1): an
// enabled-but-empty allowlist blocks every visitor — same silent-lockout
// shape as the CLI's printEmailProtection placeholder; the tool result must
// say so, not just silently succeed.
func TestRemoveEmailProtectionUserHandler_RemovingLastEntryWarns(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")

	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/myapp-a1b2/email-protect": jsonHandler(api.EmailProtection{
			EmailProtected: true,
			Emails:         []string{"a@b.com"},
		}),
		"POST /v1/apps/myapp-a1b2/email-protect": jsonHandler(api.EmailProtection{EmailProtected: true}),
	})

	result, err := removeEmailProtectionUserHandler(context.Background(), makeReq(map[string]interface{}{
		"app":    "myapp-a1b2",
		"emails": []interface{}{"a@b.com"},
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "blocks every visitor") {
		t.Errorf("expected the enabled-but-empty warning, got: %s", text)
	}
}

// TestRemoveEmailProtectionUserHandler_UnknownEntryIsNoOpSuccess (D1 open
// question, executor's call): removing an entry that isn't on the list
// succeeds without error — mirrors GET's tolerant semantics, unlike the
// CLI's remove verb which errors on a typo.
func TestRemoveEmailProtectionUserHandler_UnknownEntryIsNoOpSuccess(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")

	newMockServer(t, map[string]http.HandlerFunc{
		"GET /v1/apps/myapp-a1b2/email-protect": jsonHandler(api.EmailProtection{
			EmailProtected: true,
			Emails:         []string{"a@b.com"},
		}),
		"POST /v1/apps/myapp-a1b2/email-protect": jsonHandler(api.EmailProtection{
			EmailProtected: true,
			Emails:         []string{"a@b.com"},
		}),
	})

	result, err := removeEmailProtectionUserHandler(context.Background(), makeReq(map[string]interface{}{
		"app":    "myapp-a1b2",
		"emails": []interface{}{"zzz@unknown.com"},
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "a@b.com") {
		t.Errorf("expected the unchanged list in result, got: %s", text)
	}
}

// --- clear_email_protection (h-7b9l/h-dmd4 T-006, formula tests-first):
// directive-spelled alias of disable_email_protection — same handler
// behavior, both names callable. ---

func TestClearEmailProtectionHandler_MissingParams(t *testing.T) {
	result, err := clearEmailProtectionHandler(context.Background(), makeReq(map[string]interface{}{}))
	assertError(t, result, err, "missing required parameter")
}

func TestClearEmailProtectionHandler_Success(t *testing.T) {
	saveAndRestore(t)
	setAuthToken("tok")
	newMockServer(t, map[string]http.HandlerFunc{
		"DELETE /v1/apps/myapp-a1b2/email-protect": jsonHandler(api.EmailProtection{EmailProtected: false}),
	})

	result, err := clearEmailProtectionHandler(context.Background(), makeReq(map[string]interface{}{
		"app": "myapp-a1b2",
	}))
	text := assertSuccess(t, result, err)

	if !strings.Contains(text, "myapp-a1b2") {
		t.Errorf("expected app slug in output, got: %s", text)
	}
}
