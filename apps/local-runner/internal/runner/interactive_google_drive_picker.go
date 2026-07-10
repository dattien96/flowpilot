package runner

import (
	"errors"
	"fmt"
	"strings"
)

type googleDriveQuestionPickerToken struct {
	AccessToken string `json:"accessToken"`
	ApiKey      string `json:"apiKey"`
}

func (s *InteractiveService) GetGoogleDriveQuestionPickerToken(questionID string) (googleDriveQuestionPickerToken, error) {
	if s.runner == nil {
		return googleDriveQuestionPickerToken{}, errors.New("google drive picker is not available on this runner")
	}
	s.mu.Lock()
	rec := s.questions[strings.TrimSpace(questionID)]
	s.mu.Unlock()
	if rec == nil || rec.status != "pending" {
		return googleDriveQuestionPickerToken{}, errors.New("google drive picker question is not pending")
	}
	accessToken, err := resolveGoogleDriveAccessTokenForRunner(s.runner)
	if err != nil {
		return googleDriveQuestionPickerToken{}, err
	}
	config, err := s.runner.resolveGoogleDriveArtifactRuntimeConfig()
	if err != nil {
		return googleDriveQuestionPickerToken{}, err
	}
	return googleDriveQuestionPickerToken{
		AccessToken: accessToken,
		ApiKey:      config.PickerAPIKey,
	}, nil
}

func RenderGoogleDriveQuestionPickerHTML(questionID string) string {
	return fmt.Sprintf(`<!doctype html>
<html>
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>FlowPilot Google Drive Picker</title>
    <script src="https://apis.google.com/js/api.js"></script>
    <script src="https://accounts.google.com/gsi/client" async defer></script>
    <style>
      body { font-family: sans-serif; padding: 24px; background: #f6f7f9; color: #111827; }
      .card { max-width: 640px; margin: 32px auto; background: white; border-radius: 16px; padding: 24px; box-shadow: 0 12px 32px rgba(0,0,0,.08); }
      .muted { color: #6b7280; }
      .error { color: #b91c1c; }
      button { border: none; border-radius: 999px; padding: 12px 18px; font-weight: 600; cursor: pointer; background: #111827; color: white; }
    </style>
  </head>
  <body>
    <div class="card">
      <h1>Select a Google Drive file or folder</h1>
      <p class="muted">This selection applies only to the current run.</p>
      <p id="status">Preparing Google Picker…</p>
      <button id="retry" style="display:none">Retry picker</button>
    </div>
    <script>
      const questionId = %q;
      const pickerOrigin = window.location.protocol + "//" + window.location.host;
      const pickerRelayUrl = pickerOrigin + %q;
      const statusEl = document.getElementById("status");
      const retryButton = document.getElementById("retry");
      let pickerReady = false;
      let oauthPayload = null;

      function setStatus(message, isError) {
        statusEl.textContent = message;
        statusEl.className = isError ? "error" : "";
      }

      async function loadPicker() {
        try {
          const response = await fetch("/client/questions/" + encodeURIComponent(questionId) + "/google-drive-picker-token", { cache: "no-store" });
          const payload = await response.json();
          if (!response.ok) {
            throw new Error(payload.error?.message || payload.error || "Unable to prepare the Google Picker token.");
          }
          oauthPayload = payload;
          await new Promise((resolve) => window.gapi.load("picker", resolve));
          pickerReady = true;
          openPicker();
        } catch (error) {
          setStatus(error instanceof Error ? error.message : "Unable to prepare Google Picker.", true);
          retryButton.style.display = "inline-flex";
        }
      }

      function openPicker() {
        if (!pickerReady || !oauthPayload) return;
        retryButton.style.display = "none";
        setStatus("Waiting for Drive selection…", false);
        const folderMimeType = "application/vnd.google-apps.folder";
        const view = new google.picker.DocsView()
          .setIncludeFolders(true)
          .setSelectFolderEnabled(true);
        const picker = new google.picker.PickerBuilder()
          .setDeveloperKey(oauthPayload.apiKey)
          .setOAuthToken(oauthPayload.accessToken)
          .setOrigin(pickerOrigin)
          .setRelayUrl(pickerRelayUrl)
          .addView(view)
          .setTitle("Select a Google Drive file or folder")
          .setCallback((data) => {
            const action = data[google.picker.Response.ACTION] || data.action;
            const docs = data[google.picker.Response.DOCUMENTS] || data.docs || [];
            if (action === google.picker.Action.PICKED && docs.length > 0) {
              const target = docs[0];
              const id = target[google.picker.Document.ID] || target.id || "";
              const mimeType = target[google.picker.Document.MIME_TYPE] || target.mimeType || "";
              const kind = mimeType === folderMimeType ? "folder" : "file";
              if (window.opener) {
                window.opener.postMessage({
                  type: "flowpilot-google-drive-question-picked",
                  questionId,
                  choice: kind + ":" + id,
                }, "*");
              }
              window.setTimeout(() => window.close(), 50);
            } else if (action === google.picker.Action.CANCEL) {
              setStatus("Selection was cancelled. Retry below.", true);
              retryButton.style.display = "inline-flex";
            } else {
              setStatus("Picker returned " + String(action || "an unknown action") + ".", true);
            }
          })
          .build();
        picker.setVisible(true);
      }

      retryButton.addEventListener("click", () => {
        if (pickerReady) {
          openPicker();
        } else {
          void loadPicker();
        }
      });

      void loadPicker();
    </script>
  </body>
</html>`, questionID, googleDriveArtifactPickerRelayPath)
}
