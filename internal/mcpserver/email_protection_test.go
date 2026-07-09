package mcpserver

import (
	"context"
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
