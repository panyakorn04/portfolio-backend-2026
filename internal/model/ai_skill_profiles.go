package model

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode"
)

const (
	DefaultAISkillsDir         = "/opt/ai/skills"
	maxAISkillProfileBytes     = 16 * 1024
	maxAISkillProfileFileBytes = 8 * 1024
	maxAISelectedSkillFiles    = 2
)

type AISkillProfileStore struct {
	baseDir string
}

type aiSkillFile struct {
	Name    string
	Path    string
	Summary string
}

func NewAISkillProfileStore(baseDir string) *AISkillProfileStore {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" {
		baseDir = DefaultAISkillsDir
	}
	return &AISkillProfileStore{baseDir: baseDir}
}

func (s *AISkillProfileStore) LoadProfile(profile string) (string, error) {
	skillFiles, err := s.profileSkillFiles(profile)
	if err != nil {
		return "", err
	}
	return buildAISkillContext(skillFiles, len(skillFiles)), nil
}

func (s *AISkillProfileStore) LoadSkill(profile string, skillName string) (string, error) {
	if strings.TrimSpace(skillName) == "" || strings.Contains(skillName, "/") || strings.Contains(skillName, "\\") || strings.Contains(skillName, "..") {
		return "", fmt.Errorf("invalid AI skill name")
	}

	skillFiles, err := s.profileSkillFiles(profile)
	if err != nil {
		return "", err
	}
	for _, skill := range skillFiles {
		if skill.Name == skillName {
			return buildAISkillContext([]aiSkillFile{skill}, 1), nil
		}
	}
	return "", nil
}

func (s *AISkillProfileStore) LoadRelevantProfile(profile string, query string) (string, error) {
	skillFiles, err := s.profileSkillFiles(profile)
	if err != nil {
		return "", err
	}
	if len(skillFiles) == 0 {
		return "", nil
	}

	terms := aiSkillQueryTerms(query)
	if len(terms) == 0 {
		return buildAISkillIndex(profile, skillFiles), nil
	}

	type scoredSkill struct {
		file  aiSkillFile
		score int
	}
	scored := make([]scoredSkill, 0, len(skillFiles))
	for _, file := range skillFiles {
		score := 0
		haystack := strings.ToLower(file.Name + " " + file.Summary)
		for _, term := range terms {
			if strings.Contains(haystack, term) {
				score++
			}
		}
		if score > 0 {
			scored = append(scored, scoredSkill{file: file, score: score})
		}
	}

	if len(scored) == 0 {
		return buildAISkillIndex(profile, skillFiles), nil
	}
	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].score == scored[j].score {
			return scored[i].file.Name < scored[j].file.Name
		}
		return scored[i].score > scored[j].score
	})

	selected := make([]aiSkillFile, 0, maxAISelectedSkillFiles)
	for _, item := range scored {
		selected = append(selected, item.file)
		if len(selected) >= maxAISelectedSkillFiles {
			break
		}
	}
	return buildAISkillContext(selected, len(selected)), nil
}

func (s *AISkillProfileStore) profileSkillFiles(profile string) ([]aiSkillFile, error) {
	if s == nil {
		return nil, nil
	}
	profile = strings.TrimSpace(profile)
	if profile == "" || strings.Contains(profile, "/") || strings.Contains(profile, "\\") || strings.Contains(profile, "..") {
		return nil, fmt.Errorf("invalid AI skill profile")
	}

	profileDir := filepath.Join(s.baseDir, profile)
	entries, err := os.ReadDir(profileDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	skills := make([]aiSkillFile, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(profileDir, entry.Name(), "SKILL.md")
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			skills = append(skills, aiSkillFile{
				Name:    entry.Name(),
				Path:    path,
				Summary: readAISkillSummary(path),
			})
		}
	}
	sort.Slice(skills, func(i, j int) bool { return skills[i].Name < skills[j].Name })
	return skills, nil
}

func buildAISkillContext(skillFiles []aiSkillFile, maxFiles int) string {
	var builder strings.Builder
	for index, skill := range skillFiles {
		if index >= maxFiles || builder.Len() >= maxAISkillProfileBytes {
			break
		}
		content, err := os.ReadFile(skill.Path)
		if err != nil {
			continue
		}
		if len(content) > maxAISkillProfileFileBytes {
			content = content[:maxAISkillProfileFileBytes]
		}

		section := fmt.Sprintf("\n\n## Skill: %s\n%s", skill.Name, strings.TrimSpace(string(content)))
		remaining := maxAISkillProfileBytes - builder.Len()
		if len(section) > remaining {
			section = section[:remaining]
		}
		builder.WriteString(section)
	}
	return strings.TrimSpace(builder.String())
}

func buildAISkillIndex(profile string, skillFiles []aiSkillFile) string {
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("Skill profile %q is available. Select a specific skill only when the user's request clearly matches it. Available skills:", profile))
	for _, skill := range skillFiles {
		builder.WriteString(fmt.Sprintf("\n- %s", skill.Name))
		if skill.Summary != "" {
			builder.WriteString(": ")
			builder.WriteString(skill.Summary)
		}
	}
	return builder.String()
}

func readAISkillSummary(path string) string {
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "description:") {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line, "description:")), "\"'")
		}
	}
	return ""
}

func aiSkillQueryTerms(query string) []string {
	query = strings.ToLower(query)
	fields := strings.FieldsFunc(query, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.IsPunct(r) || unicode.IsSymbol(r)
	})
	seen := map[string]bool{}
	terms := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if len([]rune(field)) < 3 || seen[field] {
			continue
		}
		seen[field] = true
		terms = append(terms, field)
	}
	return terms
}
