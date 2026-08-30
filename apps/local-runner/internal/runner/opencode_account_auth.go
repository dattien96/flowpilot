package runner

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type opencodeAuthEntry struct {
	Type    string `json:"type"`
	Key     string `json:"key"`
	Access  string `json:"access"`
	Refresh string `json:"refresh"`
}

func firstOpencodeAuthFile(homePath string) string {
	for _, path := range opencodeAuthFilePaths(homePath) {
		if fileExists(path) {
			return path
		}
	}
	return ""
}

func opencodeProviderDisplayName(key string) string {
	switch strings.ToLower(strings.TrimSpace(key)) {
	case "opencode":
		return "OpenCode Zen"
	case "opencode-go":
		return "OpenCode Go"
	case "xai":
		return "xAI"
	default:
		key = strings.TrimSpace(key)
		if key == "" {
			return ""
		}
		return strings.ToUpper(key[:1]) + key[1:]
	}
}

func loadOpencodeConnectedProvidersFromAuth(homePath string) (labels []string, email string) {
	authPath := firstOpencodeAuthFile(homePath)
	if authPath == "" {
		return nil, ""
	}
	data, err := os.ReadFile(authPath)
	if err != nil {
		return nil, ""
	}
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, ""
	}
	keys := make([]string, 0, len(payload))
	for key := range payload {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	labels = make([]string, 0, len(keys))
	for _, key := range keys {
		label := opencodeProviderDisplayName(key)
		if label == "" {
			continue
		}
		labels = append(labels, label)
		if email == "" && strings.EqualFold(key, "xai") {
			var entry opencodeAuthEntry
			if err := json.Unmarshal(payload[key], &entry); err == nil {
				email = emailFromJWTAccessToken(entry.Access)
			}
		}
	}
	return labels, email
}

func emailFromJWTAccessToken(access string) string {
	access = strings.TrimSpace(access)
	parts := strings.Split(access, ".")
	if len(parts) < 2 {
		return ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return ""
	}
	var claims struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return ""
	}
	return strings.TrimSpace(claims.Email)
}

func parseOpencodeProvidersListOutput(raw []byte) (labels []string, email string) {
	text := strings.TrimSpace(string(raw))
	if text == "" {
		return nil, ""
	}
	labels = make([]string, 0, 4)
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(stripANSI(line))
		if line == "" {
			continue
		}
		if strings.Contains(line, "Credentials") || strings.HasPrefix(line, "└") || strings.HasPrefix(line, "┌") {
			continue
		}
		line = strings.TrimPrefix(line, "●")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		label := fields[0]
		if len(fields) > 1 && strings.EqualFold(fields[0], "OpenCode") {
			label = fields[0] + " " + fields[1]
		}
		if label != "" {
			labels = append(labels, label)
		}
	}
	return labels, email
}

func stripANSI(value string) string {
	var b strings.Builder
	b.Grow(len(value))
	esc := false
	for i := 0; i < len(value); i++ {
		ch := value[i]
		if esc {
			if ch == 'm' {
				esc = false
			}
			continue
		}
		if ch == '\x1b' {
			esc = true
			continue
		}
		b.WriteByte(ch)
	}
	return b.String()
}

func opencodeAccountDisplayLabel(labels []string, email string) string {
	if strings.TrimSpace(email) != "" {
		return strings.TrimSpace(email)
	}
	if len(labels) > 0 {
		return strings.Join(labels, " · ")
	}
	return "OpenCode"
}

func opencodeAccountUsageSummary(labels []string) string {
	if len(labels) == 0 {
		return ""
	}
	return fmt.Sprintf("%d connected provider(s): %s", len(labels), strings.Join(labels, ", "))
}
