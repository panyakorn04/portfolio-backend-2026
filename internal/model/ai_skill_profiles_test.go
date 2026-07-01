package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAISkillProfileStoreLoadRelevantProfileSelectsMatchingSkill(t *testing.T) {
	skillsDir := t.TempDir()
	writeTestSkill(t, skillsDir, "ai-console", "vps-ai-services", "Use when answering VPS and Docker deployment questions.", "VPS DETAILS")
	writeTestSkill(t, skillsDir, "ai-console", "youtube-highlight-automation", "Use when answering YouTube highlight automation questions.", "YOUTUBE DETAILS")

	store := NewAISkillProfileStore(skillsDir)
	context, err := store.LoadRelevantProfile("ai-console", "ช่วยดู VPS docker deploy ให้หน่อย")
	if err != nil {
		t.Fatalf("LoadRelevantProfile returned error: %v", err)
	}
	if !strings.Contains(context, "VPS DETAILS") {
		t.Fatalf("expected matching VPS skill in context: %s", context)
	}
	if strings.Contains(context, "YOUTUBE DETAILS") {
		t.Fatalf("did not expect unrelated YouTube skill in context: %s", context)
	}
}

func TestAISkillProfileStoreLoadRelevantProfileFallsBackToIndex(t *testing.T) {
	skillsDir := t.TempDir()
	writeTestSkill(t, skillsDir, "ai-console", "vps-ai-services", "Use when answering VPS and Docker deployment questions.", "VPS DETAILS")
	writeTestSkill(t, skillsDir, "ai-console", "youtube-highlight-automation", "Use when answering YouTube highlight automation questions.", "YOUTUBE DETAILS")

	store := NewAISkillProfileStore(skillsDir)
	context, err := store.LoadRelevantProfile("ai-console", "สวัสดี")
	if err != nil {
		t.Fatalf("LoadRelevantProfile returned error: %v", err)
	}
	if !strings.Contains(context, "Available skills") || !strings.Contains(context, "vps-ai-services") || !strings.Contains(context, "youtube-highlight-automation") {
		t.Fatalf("expected compact skill index: %s", context)
	}
	if strings.Contains(context, "VPS DETAILS") || strings.Contains(context, "YOUTUBE DETAILS") {
		t.Fatalf("expected index without full skill body: %s", context)
	}
}

func writeTestSkill(t *testing.T, baseDir, profile, skill, description, body string) {
	t.Helper()
	dir := filepath.Join(baseDir, profile, skill)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\nname: " + skill + "\ndescription: " + description + "\n---\n\n# " + skill + "\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
