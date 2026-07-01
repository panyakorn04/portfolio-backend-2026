package model

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	DefaultAISkillsDir         = "/opt/ai/skills"
	maxAISkillProfileBytes     = 16 * 1024
	maxAISkillProfileFileBytes = 8 * 1024
)

type AISkillProfileStore struct {
	baseDir string
}

func NewAISkillProfileStore(baseDir string) *AISkillProfileStore {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		baseDir = DefaultAISkillsDir
	}
	return &AISkillProfileStore{baseDir: baseDir}
}

func (s *AISkillProfileStore) LoadProfile(profile string) (string, error) {
	if s == nil {
		return "", nil
	}
	profile = strings.TrimSpace(profile)
	if profile == "" || strings.Contains(profile, "/") || strings.Contains(profile, "\\") || strings.Contains(profile, "..") {
		return "", fmt.Errorf("invalid AI skill profile")
	}

	profileDir := filepath.Join(s.baseDir, profile)
	entries, err := os.ReadDir(profileDir)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	var skillFiles []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(profileDir, entry.Name(), "SKILL.md")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			skillFiles = append(skillFiles, path)
		}
	}
	sort.Strings(skillFiles)

	var builder strings.Builder
	for _, path := range skillFiles {
		if builder.Len() >= maxAISkillProfileBytes {
			break
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		if len(content) > maxAISkillProfileFileBytes {
			content = content[:maxAISkillProfileFileBytes]
		}

		skillName := filepath.Base(filepath.Dir(path))
		section := fmt.Sprintf("\n\n## Skill: %s\n%s", skillName, strings.TrimSpace(string(content)))
		remaining := maxAISkillProfileBytes - builder.Len()
		if len(section) > remaining {
			section = section[:remaining]
		}
		builder.WriteString(section)
	}

	return strings.TrimSpace(builder.String()), nil
}
