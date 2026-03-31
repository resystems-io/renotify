package command

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go"

	"go.resystems.io/renotify/internal/exitcode"
	"go.resystems.io/renotify/internal/payload"
)

func TestAsk_ResponseBoolean(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	// Subscriber that watches for the request and replies with
	// a NotificationResponse on the .response subject.
	nc, _ := nats.Connect(srv.ClientURL())
	defer nc.Close()

	nc.Subscribe("resystems.renotify.testuser.flow.>", func(msg *nats.Msg) {
		if !strings.HasSuffix(msg.Subject, ".request") {
			return
		}
		var req payload.NotificationRequest
		json.Unmarshal(msg.Data, &req)

		// Publish response after a short delay.
		go func() {
			time.Sleep(50 * time.Millisecond)
			respSubj := strings.TrimSuffix(msg.Subject, ".request") + ".response"
			resp := payload.NotificationResponse{
				RequestID: req.ID,
				Accepted:  boolPtr(true),
				Timestamp: time.Now().UTC(),
			}
			data, _ := json.Marshal(resp)
			respMsg := &nats.Msg{
				Subject: respSubj,
				Data:    data,
				Header:  nats.Header{},
			}
			respMsg.Header.Set("Nats-Msg-Id", req.ID+"-resp")
			js, _ := nc.JetStream()
			js.PublishMsg(respMsg)
		}()
	})

	stdout, _, err := executeCommand("ask",
		"-t", "Deploy?",
		"-r", "boolean",
		"--timeout", "5s",
		"--format", "json",
	)
	if err != nil {
		t.Fatalf("ask failed: %v", err)
	}

	var resp payload.NotificationResponse
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if resp.Accepted == nil || !*resp.Accepted {
		t.Error("expected accepted=true")
	}
}

func TestAsk_ResponseText(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	nc, _ := nats.Connect(srv.ClientURL())
	defer nc.Close()

	nc.Subscribe("resystems.renotify.testuser.flow.>", func(msg *nats.Msg) {
		if !strings.HasSuffix(msg.Subject, ".request") {
			return
		}
		var req payload.NotificationRequest
		json.Unmarshal(msg.Data, &req)

		go func() {
			time.Sleep(50 * time.Millisecond)
			respSubj := strings.TrimSuffix(msg.Subject, ".request") + ".response"
			resp := payload.NotificationResponse{
				RequestID: req.ID,
				Accepted:  boolPtr(false),
				Text:      "Wait for audit",
				Timestamp: time.Now().UTC(),
			}
			data, _ := json.Marshal(resp)
			respMsg := &nats.Msg{Subject: respSubj, Data: data, Header: nats.Header{}}
			respMsg.Header.Set("Nats-Msg-Id", req.ID+"-resp")
			js, _ := nc.JetStream()
			js.PublishMsg(respMsg)
		}()
	})

	stdout, _, err := executeCommand("ask",
		"-t", "Proceed?",
		"-r", "boolean,text",
		"--timeout", "5s",
		"--format", "text",
	)
	if err != nil {
		t.Fatalf("ask failed: %v", err)
	}
	if !strings.Contains(stdout, "Response: No") {
		t.Errorf("missing 'Response: No' in %q", stdout)
	}
	if !strings.Contains(stdout, "Comment:  Wait for audit") {
		t.Errorf("missing comment in %q", stdout)
	}
}

func TestAsk_ErrorTimeout(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	// Override grace period to keep test fast.
	t.Setenv("RENOTIFY_TIMEOUT_ASK_GRACE_PERIOD", "1s")

	nc, _ := nats.Connect(srv.ClientURL())
	defer nc.Close()

	// Publish a daemon-sourced timeout ErrorResponse.
	nc.Subscribe("resystems.renotify.testuser.flow.>", func(msg *nats.Msg) {
		if !strings.HasSuffix(msg.Subject, ".request") {
			return
		}
		var req payload.NotificationRequest
		json.Unmarshal(msg.Data, &req)

		go func() {
			time.Sleep(50 * time.Millisecond)
			respSubj := strings.TrimSuffix(msg.Subject, ".request") + ".response"
			errResp := payload.ErrorResponse{
				CorrelationID: req.ID,
				Code:          "timeout",
				Message:       "No response received within 5s.",
				Timestamp:     time.Now().UTC(),
			}
			data, _ := json.Marshal(errResp)
			respMsg := &nats.Msg{Subject: respSubj, Data: data, Header: nats.Header{}}
			respMsg.Header.Set("Nats-Msg-Id", req.ID+"-timeout")
			js, _ := nc.JetStream()
			js.PublishMsg(respMsg)
		}()
	})

	_, _, err := executeCommand("ask",
		"-t", "Deploy?",
		"-r", "boolean",
		"--timeout", "5s",
	)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	ce, ok := err.(*exitcode.CodedError)
	if !ok {
		t.Fatalf("expected CodedError, got %T: %v", err, err)
	}
	if ce.Code != exitcode.Timeout {
		t.Errorf("exit code = %d, want %d", ce.Code, exitcode.Timeout)
	}
}

func TestAsk_InterjectionStop(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)
	t.Setenv("RENOTIFY_TIMEOUT_ASK_GRACE_PERIOD", "1s")

	nc, _ := nats.Connect(srv.ClientURL())
	defer nc.Close()

	nc.Subscribe("resystems.renotify.testuser.flow.>", func(msg *nats.Msg) {
		if !strings.HasSuffix(msg.Subject, ".request") {
			return
		}
		go func() {
			time.Sleep(50 * time.Millisecond)
			interjSubj := strings.TrimSuffix(msg.Subject, ".request") + ".interject"
			interj := payload.InterjectionCommand{
				FlowID:    "",
				Action:    payload.InterjectionStop,
				Timestamp: time.Now().UTC(),
			}
			data, _ := json.Marshal(interj)
			interjMsg := &nats.Msg{Subject: interjSubj, Data: data, Header: nats.Header{}}
			interjMsg.Header.Set("Nats-Msg-Id", "stop-1")
			js, _ := nc.JetStream()
			js.PublishMsg(interjMsg)
		}()
	})

	_, stderr, err := executeCommand("ask",
		"-t", "Deploy?",
		"-r", "boolean",
		"--timeout", "5s",
	)
	if err == nil {
		t.Fatal("expected stop error")
	}
	if !strings.Contains(err.Error(), "stopped by user") {
		t.Errorf("error = %q, expected 'stopped by user'", err)
	}
	_ = stderr
}

func TestAsk_InterjectionNote(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)
	t.Setenv("RENOTIFY_TIMEOUT_ASK_GRACE_PERIOD", "1s")

	nc, _ := nats.Connect(srv.ClientURL())
	defer nc.Close()

	nc.Subscribe("resystems.renotify.testuser.flow.>", func(msg *nats.Msg) {
		if !strings.HasSuffix(msg.Subject, ".request") {
			return
		}
		var req payload.NotificationRequest
		json.Unmarshal(msg.Data, &req)

		go func() {
			time.Sleep(50 * time.Millisecond)
			flowBase := strings.TrimSuffix(msg.Subject, ".request")

			// Send a note interjection first.
			interj := payload.InterjectionCommand{
				Action:    payload.InterjectionNote,
				Context:   "FYI: staging is down",
				Timestamp: time.Now().UTC(),
			}
			data, _ := json.Marshal(interj)
			interjMsg := &nats.Msg{Subject: flowBase + ".interject", Data: data, Header: nats.Header{}}
			interjMsg.Header.Set("Nats-Msg-Id", "note-1")
			js, _ := nc.JetStream()
			js.PublishMsg(interjMsg)

			// Then send the response.
			time.Sleep(50 * time.Millisecond)
			resp := payload.NotificationResponse{
				RequestID: req.ID,
				Accepted:  boolPtr(true),
				Timestamp: time.Now().UTC(),
			}
			data, _ = json.Marshal(resp)
			respMsg := &nats.Msg{Subject: flowBase + ".response", Data: data, Header: nats.Header{}}
			respMsg.Header.Set("Nats-Msg-Id", req.ID+"-resp")
			js.PublishMsg(respMsg)
		}()
	})

	stdout, stderr, err := executeCommand("ask",
		"-t", "Continue?",
		"-r", "boolean",
		"--timeout", "5s",
	)
	if err != nil {
		t.Fatalf("ask failed: %v", err)
	}

	// Note should appear on stderr.
	if !strings.Contains(stderr, "FYI: staging is down") {
		t.Errorf("stderr missing note: %q", stderr)
	}

	// Response should be on stdout.
	var resp payload.NotificationResponse
	if err := json.Unmarshal([]byte(stdout), &resp); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout)
	}
	if resp.Accepted == nil || !*resp.Accepted {
		t.Error("expected accepted=true")
	}
}

func TestAsk_SafetyTimeout(t *testing.T) {
	srv, stateDir := startTestNATS(t)
	defer srv.Shutdown()
	setupPostEnv(t, srv, stateDir)

	// Very short timeout + grace to keep test fast.
	t.Setenv("RENOTIFY_TIMEOUT_ASK_GRACE_PERIOD", "1s")

	// No subscriber to respond — the safety timer should fire.
	_, _, err := executeCommand("ask",
		"-t", "Deploy?",
		"-r", "boolean",
		"--timeout", "1s",
	)
	if err == nil {
		t.Fatal("expected safety timeout error")
	}
	ce, ok := err.(*exitcode.CodedError)
	if !ok {
		t.Fatalf("expected CodedError, got %T: %v", err, err)
	}
	if ce.Code != exitcode.Timeout {
		t.Errorf("exit code = %d, want %d", ce.Code, exitcode.Timeout)
	}
	if !strings.Contains(err.Error(), "daemon did not respond") {
		t.Errorf("error = %q, expected 'daemon did not respond'", err)
	}
}

func TestAsk_ValidationMissingTitle(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, _, err := executeCommand("ask", "-r", "boolean")
	if err == nil {
		t.Fatal("expected error for missing title")
	}
}

func TestAsk_ValidationMissingResponseTypes(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, _, err := executeCommand("ask", "-t", "Deploy?")
	if err == nil {
		t.Fatal("expected error for missing response-types")
	}
}

func TestAsk_ValidationChoiceWithoutActions(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, _, err := executeCommand("ask",
		"-t", "Choose",
		"-r", "choice",
	)
	if err == nil {
		t.Fatal("expected error for choice without actions")
	}
	if !strings.Contains(err.Error(), "--actions is required") {
		t.Errorf("error = %q", err)
	}
}

func TestAsk_ValidationInvalidResponseType(t *testing.T) {
	t.Setenv("RENOTIFY_USERNAME", "testuser")
	_, _, err := executeCommand("ask",
		"-t", "Test",
		"-r", "invalid",
	)
	if err == nil {
		t.Fatal("expected error for invalid response type")
	}
	if !strings.Contains(err.Error(), "invalid response type") {
		t.Errorf("error = %q", err)
	}
}

// boolPtr returns a pointer to a bool value.
func boolPtr(v bool) *bool { return &v }
