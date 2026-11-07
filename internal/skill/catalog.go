package skill

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const maxSkillFileSize = 24 * 1024

var skillNamePattern = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

type Skill struct {
	Name        string
	Description string
}

type Catalog struct {
	skills []Skill
}

func LoadCatalog(root string) (Catalog, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return Catalog{}, err
	}

	catalog := Catalog{skills: make([]Skill, 0, len(entries))}
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.IsDir() {
			continue
		}

		skill, err := loadSkill(root, entry.Name())
		if err != nil {
			log.Printf("skip skill %q: %v", entry.Name(), err)
			continue
		}

		catalog.skills = append(catalog.skills, skill)
	}

	return catalog, nil
}

func loadSkill(root, name string) (Skill, error) {
	skillPath := filepath.Join(root, name, "SKILL.md")
	info, err := os.Lstat(skillPath)
	if err != nil {
		return Skill{}, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return Skill{}, errors.New("SKILL.md is not a regular file")
	}
	if info.Size() > maxSkillFileSize {
		return Skill{}, fmt.Errorf("SKILL.md exceeds %d bytes", maxSkillFileSize)
	}

	content, err := os.ReadFile(skillPath)
	if err != nil {
		return Skill{}, err
	}
	return parseSkill(name, content)
}

func parseSkill(dirName string, content []byte) (Skill, error) {
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")

	if len(lines) == 0 || lines[0] != "---" {
		return Skill{}, errors.New("missing frontmatter start")
	}

	skill := Skill{}
	for _, line := range lines[1:] {
		if line == "---" {
			if skill.Name == "" || skill.Name != dirName || !skillNamePattern.MatchString(skill.Name) {
				return Skill{}, fmt.Errorf("invalid skill name %q", skill.Name)
			}
			if skill.Description == "" {
				return Skill{}, errors.New("missing skill description")
			}
			return skill, nil
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "name":
			skill.Name = value
		case "description":
			skill.Description = value
		}
	}
	return Skill{}, errors.New("missing frontmatter end")
}

func (c Catalog) Prompt() string {
	if len(c.skills) == 0 {
		return ""
	}

	lines := []string{
		"以下是可用技能：",
	}
	for _, skill := range c.skills {
		lines = append(lines, fmt.Sprintf("- %s: %s", skill.Name, skill.Description))
	}
	lines = append(lines, "技能位于只读目录 /skills，匹配技能后，可通过 terminal_exec 读取 /skills/<name>/ 下的文件。")

	return strings.Join(lines, "\n")
}
