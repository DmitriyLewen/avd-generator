package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"text/template"

	"github.com/aquasecurity/avd-generator/menu"
	"github.com/aquasecurity/avd-generator/util"
)

var (
	SeverityNames = []string{
		"info",
		"low",
		"medium",
		"high",
		"critical",
	}
)

func getSeverityName(sev string) string {
	sevIndex, _ := strconv.Atoi(sev)
	return SeverityNames[sevIndex]
}

func findSubstringsInString(target string, needles []string) bool {
	for _, s := range needles {
		if strings.Contains(target, s) {
			return true
		}
	}
	return false
}

func findSuffixSubstringInString(target string, needles []string) bool {
	for _, s := range needles {
		if strings.HasSuffix(target, s) {
			return true
		}
	}
	return false
}

type Signature struct {
	ID      string
	Version string
	Name    string

	Description string
	Severity    string
	MitreAttack string
	// Tags        []string
	// Properties map[string]interface{}
	RegoPolicy string
	GoCode     string
}

type TraceePost struct {
	Title      string
	Date       string
	TopLevelID string
	ParentID   string
	ParentName string
	AliasID    string
	Signature  Signature
}

func TraceePostToMarkdown(tp TraceePost, outputFile *os.File) error {
	t := template.Must(template.New("traceePost").Parse(signaturePostTemplate))
	err := t.Execute(outputFile, tp)
	if err != nil {
		return err
	}
	return nil
}

func generateTraceePages(rulesDir, postsDir string, clock Clock) error {
	err := os.MkdirAll(postsDir, 0755)
	if err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	log.Println("generating tracee pages in: ", postsDir)

	if err := generateGoSigPages(rulesDir, postsDir, clock); err != nil {
		return fmt.Errorf("generate go sig pages: %w", err)
	}
	return nil
}

func generateGoSigPages(rulesDir string, postsDir string, clock Clock) error {
	var files []string
	var err error
	if files, err = getAllFiles(rulesDir); err != nil {
		return err
	}

	for _, file := range files {
		func(file string) {
			defer func() {
				if err := recover(); err != nil {
					log.Printf("failed to process file %s: %v", file, err)
				}
			}()

			if findSubstringsInString(file, []string{"helpers.go", "example.go", "export.go", "traceerego.go", "aio", "common", "mapper"}) || findSuffixSubstringInString(file, []string{".md", ".rego", "test.go", ".disabled"}) {
				return
			}

			b, _ := os.ReadFile(file)
			r := strings.NewReplacer(`"`, ``)
			rTitle := strings.NewReplacer("/", "-", `"`, "")

			log.Printf("Processing Tracee go signature file: %s", file)

			mitreAttack := r.Replace(getRegexMatch(`\"(MITRE ATT&CK)\"\:\s*\"(.*)\"`, string(b)))
			category := r.Replace(getRegexMatch(`\"(Category)\"\:\s*\"(.*)\"`, string(b)))
			technique := r.Replace(getRegexMatch(`\"(Technique)\"\:\s*\"(.*)\"`, string(b)))

			if mitreAttack == "" {
				mitreAttack = fmt.Sprintf("%s: %s", strings.Title(strings.ReplaceAll(category, "-", " ")), technique)
			}

			sig := Signature{
				ID:          r.Replace(getRegexMatch(`(ID)\:\s*\"(.*?)"`, string(b))),
				Version:     r.Replace(getRegexMatch(`(Version)\:\s*\"(.*?)\"`, string(b))),
				Name:        rTitle.Replace(getRegexMatch(`(Name)\:\s*\"(.*?)\"`, string(b))),
				Description: r.Replace(getRegexMatch(`(Description)\:\s*\"(.*?)\"`, string(b))),
				Severity:    getSeverityName(getRegexMatch(`\"(Severity)\"\:\s*\d`, string(b))),
				MitreAttack: mitreAttack,
				GoCode:      string(b),
			}

			topLevelIDName := strings.TrimSpace(strings.Split(sig.MitreAttack, ":")[0])
			topLevelID := strings.ToLower(strings.ReplaceAll(topLevelIDName, " ", "-"))
			runTimeSecurityMenu.AddNode(topLevelID, strings.Title(topLevelIDName), postsDir, "", []string{"runtime"}, []menu.BreadCrumb{
				{Name: "Runtime Security", Url: "/tracee"},
			}, "runtime", true)
			parentID := topLevelID

			outputFilepath := filepath.Join(postsDir, parentID, fmt.Sprintf("%s.md", strings.ReplaceAll(sig.ID, "-", "")))
			if err := os.MkdirAll(filepath.Dir(outputFilepath), 0755); err != nil {
				log.Printf("error occurred while creating target directory: %s, %s", filepath.Dir(outputFilepath), err)
			}

			f, err := os.Create(outputFilepath)
			if err != nil {
				log.Printf("unable to create tracee markdown file: %s for sig: %s, skipping...\n", err, sig.ID)
				return
			}

			if err = TraceePostToMarkdown(TraceePost{
				Title:      util.Nicify(strings.Title(strings.ReplaceAll(sig.Name, "-", " "))),
				TopLevelID: parentID,
				ParentID:   parentID,
				ParentName: strings.Title(topLevelIDName),
				AliasID:    strings.ToLower(strings.ReplaceAll(sig.ID, "-", "")),
				Date:       clock.Now("2006-01-02"),
				Signature:  sig,
			}, f); err != nil {
				log.Printf("unable to write tracee signature markdown: %s.md, err: %s", sig.ID, err)
				return
			}
		}(file)
	}
	return nil
}

func getRegexMatch(regex, str string) string {
	result := regexp.MustCompile(regex).FindString(str)
	if result == "" {
		return ""
	}
	parts := strings.SplitN(result, ":", 2)
	if len(parts) < 2 {
		return ""
	}

	return strings.TrimSpace(parts[1])
}

const signaturePostTemplate = `---
title: {{.Title}}
id: {{.Signature.ID}}
aliases: [
    "/tracee/{{ .AliasID}}"
]
source: Tracee
icon: aqua
shortName: {{.Title}}
severity: {{.Signature.Severity}}
draft: false
version: {{.Signature.Version}}
keywords: "{{.Signature.ID}}"

category: runsec
date: {{.Date}}

remediations:

breadcrumbs: 
  - name: Tracee
    path: /tracee
  - name: {{ .ParentName }}
    path: /tracee/{{ .ParentID }}

avd_page_type: avd_page
---

### {{.Title}}
{{.Signature.Description}}

### MITRE ATT&CK
{{.Signature.MitreAttack}}


{{if .Signature.RegoPolicy}}### Rego Policy
` + "```\n{{ .Signature.RegoPolicy }}\n```" + `
{{- end}}
{{- if .Signature.GoCode}}### Go Source
` + "```\n{{ .Signature.GoCode }}\n```" + `
{{- end}}
`
