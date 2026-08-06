package config

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"strings"
)

// INIFile INI configuration file
type INIFile struct {
	path    string
	lines   []iniLine // Keep original line order, including comments
	section map[string]int
}

type iniLine struct {
	isComment bool
	comment   string
	section   string // Non-empty means this is a [section] line
	key       string
	value     string
	rawLine   string // Original line content (for unparsed lines)
}

// LoadINI loads the INI configuration file from the file
func LoadINI(path string) (*INIFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("打开配置文件失败: %w", err)
	}
	ini, err := LoadINIBytes(data)
	if err != nil {
		return nil, err
	}
	ini.path = path
	return ini, nil
}

// LoadINIBytes parses the INI configuration file from the byte content (used to read the default configuration embedded at compile time)
func LoadINIBytes(data []byte) (*INIFile, error) {
	ini := &INIFile{
		section: make(map[string]int),
	}

	currentSection := ""
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		raw := scanner.Text()
		line := strings.TrimSpace(raw)

		// Blank line
		if line == "" {
			ini.lines = append(ini.lines, iniLine{rawLine: raw})
			continue
		}

		//Comment line
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			ini.lines = append(ini.lines, iniLine{isComment: true, comment: raw})
			continue
		}

		// Section row
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			currentSection = strings.TrimSpace(line[1 : len(line)-1])
			ini.section[currentSection] = len(ini.lines)
			ini.lines = append(ini.lines, iniLine{section: currentSection, rawLine: raw})
			continue
		}

		// Key = Value line
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			//remove quotes
			if len(val) >= 2 && (val[0] == '"' && val[len(val)-1] == '"') {
				val = val[1 : len(val)-1]
			}
			ini.lines = append(ini.lines, iniLine{
				section: currentSection,
				key:     key,
				value:   val,
				rawLine: raw,
			})
			continue
		}

		// Leave other rows as is
		ini.lines = append(ini.lines, iniLine{rawLine: raw})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("读取配置文件失败: %w", err)
	}

	return ini, nil
}

// Get gets the key value under the specified section
func (f *INIFile) Get(section, key string) string {
	for _, l := range f.lines {
		if l.section == section && l.key == key {
			return l.value
		}
	}
	return ""
}

// Set sets the key value under the specified section, and appends it if it does not exist
func (f *INIFile) Set(section, key, value string) {
	for i, l := range f.lines {
		if l.section == section && l.key == key {
			f.lines[i].value = value
			return
		}
	}

	// key does not exist and needs to be appended to the end of the corresponding section
	if _, ok := f.section[section]; !ok {
		// section does not exist, append new section
		f.lines = append(f.lines, iniLine{section: section, rawLine: "[" + section + "]"})
		f.section[section] = len(f.lines) - 1
	}

	// Find the position of the last key after section and insert it after it
	insertAt := len(f.lines)
	for i, l := range f.lines {
		if l.section == section && l.key != "" {
			insertAt = i + 1
		}
	}

	//Insert new row
	newLine := iniLine{section: section, key: key, value: value}
	if insertAt >= len(f.lines) {
		f.lines = append(f.lines, newLine)
	} else {
		f.lines = append(f.lines[:insertAt], append([]iniLine{newLine}, f.lines[insertAt:]...)...)
	}
}

// Save writes the configuration back to the file
func (f *INIFile) Save() error {
	var sb strings.Builder
	for _, l := range f.lines {
		if l.isComment {
			sb.WriteString(l.comment)
		} else if l.section != "" && l.key == "" {
			// Section row
			sb.WriteString(l.rawLine)
		} else if l.key != "" {
			// Key = Value line
			sb.WriteString(fmt.Sprintf("%s = %s", l.key, l.value))
		} else {
			// Blank line or other
			sb.WriteString(l.rawLine)
		}
		sb.WriteString("\n")
	}
	return os.WriteFile(f.path, []byte(sb.String()), 0644)
}

// Path returns the configuration file path
func (f *INIFile) Path() string {
	return f.path
}

// SetPath sets the configuration file path (for saving; the embedded default configuration can specify the runtime disk path)
func (f *INIFile) SetPath(p string) {
	f.path = p
}
