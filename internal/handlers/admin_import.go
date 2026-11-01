package handlers

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/openlyfree/gopher-ctf/internal/models"
	"github.com/openlyfree/gopher-ctf/internal/shared"
	"github.com/openlyfree/gopher-ctf/ui/pages"
	"gorm.io/gorm"
)

const ( maxImportBytes int64 = 2 << 20; maxImportRows = 500 )

type importRow struct { Number int; Challenge models.Challenge }
type importPlan struct { row importRow; existing *models.Challenge; action string }

func AdminImportPageHandler(c *gin.Context) { c.Header("Cache-Control", "no-store"); Render(c, 200, pages.AdminImport()) }

func AdminImportHandler(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxImportBytes)
	file, err := importFile(c)
	if err != nil { c.String(400, "Please upload a JSON file"); return }
	data, err := io.ReadAll(io.LimitReader(file, maxImportBytes+1))
	if err != nil { c.String(400, "Could not read the import file"); return }
	if int64(len(data)) > maxImportBytes { c.String(400, "Import file is too large"); return }
	rows, err := decodeImport(data)
	if err != nil { c.String(400, "Invalid import JSON: "+err.Error()); return }
	if len(rows) > maxImportRows { c.String(400, "Import contains too many challenges (maximum is 500)"); return }

	update := c.PostForm("update_existing") == "on" || c.PostForm("update_existing") == "1" || c.PostForm("update_existing") == "true"
	plans := make([]importPlan, 0, len(rows)); names := map[string]bool{}
	for i, input := range rows {
		challenge, validationErr := validateChallengeInput(input)
		if validationErr != nil {
			message := validationErr.Error(); if validationErr == errChallengeRequired { message = "all fields are required" }
			c.String(400, fmt.Sprintf("#%d '%s': %s", i+1, challengeName(input), message)); return
		}
		key := strings.ToLower(challenge.Title)
		if names[key] { c.String(400, fmt.Sprintf("#%d '%s': duplicate name in file", i+1, challenge.Title)); return }
		names[key] = true
		var existing models.Challenge
		result := shared.DB.Where("LOWER(title) = LOWER(?)", challenge.Title).First(&existing)
		if result.Error != nil && !errors.Is(result.Error, gorm.ErrRecordNotFound) { _ = c.Error(result.Error); c.String(500, "Database error"); return }
		plan := importPlan{row: importRow{Number:i+1, Challenge:challenge}, action:"created"}
		if result.Error == nil { plan.existing = &existing; if update { plan.action = "updated" } else { plan.action = "skipped" } }
		plans = append(plans, plan)
	}

	dryRun := c.PostForm("dry_run") == "on" || c.PostForm("dry_run") == "1" || c.PostForm("dry_run") == "true"
	if !dryRun {
		err = shared.DB.Transaction(func(tx *gorm.DB) error {
			for _, plan := range plans {
				if plan.action == "skipped" { continue }
				if plan.action == "updated" { if err := tx.Model(&models.Challenge{}).Where("id = ?", plan.existing.ID).Updates(map[string]interface{}{"description":plan.row.Challenge.Description,"points":plan.row.Challenge.Points,"flag":plan.row.Challenge.Flag,"hints":plan.row.Challenge.Hints}).Error; err != nil { return err }; continue }
				if err := tx.Create(&plan.row.Challenge).Error; err != nil { return err }
			}
			return nil
		})
		if err != nil { _ = c.Error(err); c.String(500, "Database error"); return }
	}
	created, updated, skipped := 0, 0, 0
	for _, plan := range plans { switch plan.action { case "created": created++; case "updated": updated++; case "skipped": skipped++ } }
	prefix := ""
	if dryRun { prefix = "Dry run: " }
	c.String(200, "%s%d created, %d updated, %d skipped", prefix, created, updated, skipped)
}

func challengeName(input challengeInput) string { return strings.TrimSpace(input.Name) }

func importFile(c *gin.Context) (io.Reader, error) {
	if file, err := c.FormFile("import_file"); err == nil { return file.Open() }
	if err := c.Request.ParseMultipartForm(maxImportBytes); err != nil { return nil, err }
	for _, files := range c.Request.MultipartForm.File { if len(files) > 0 { return files[0].Open() } }
	return nil, errors.New("missing file")
}

func decodeImport(data []byte) ([]challengeInput, error) {
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 || trimmed[0] != '[' { return nil, errors.New("top level must be an array of objects") }
	decoder := json.NewDecoder(bytes.NewReader(data)); decoder.UseNumber()
	var raw []json.RawMessage
	if err := decoder.Decode(&raw); err != nil { return nil, errors.New("top level must be an array of objects") }
	var extra json.RawMessage
	if err := decoder.Decode(&extra); err != io.EOF { return nil, errors.New("trailing JSON is not allowed") }
	inputs := make([]challengeInput, 0, len(raw))
	for _, item := range raw {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(item, &fields); err != nil || fields == nil { return nil, errors.New("each entry must be an object") }
		input, err := decodeImportRow(fields); if err != nil { return nil, err }; inputs = append(inputs, input)
	}
	return inputs, nil
}

func decodeImportRow(fields map[string]json.RawMessage) (challengeInput, error) {
	getString := func(name string, required bool) (string, error) { raw, ok := fields[name]; if !ok { if required { return "", errChallengeRequired }; return "", nil }; var value string; if err := json.Unmarshal(raw, &value); err != nil { return "", fmt.Errorf("%s must be a string", name) }; return value, nil }
	name, err := getString("challenge_name", true); if err != nil { return challengeInput{}, err }
	description, err := getString("challenge_description", true); if err != nil { return challengeInput{}, err }
	flag, err := getString("challenge_flag", true); if err != nil { return challengeInput{}, err }
	hints := ""; if raw, ok := fields["challenge_hints"]; ok { trimmed := bytes.TrimSpace(raw); if len(trimmed) > 0 && trimmed[0] == '[' { var list []string; if err := json.Unmarshal(raw, &list); err != nil { return challengeInput{}, errors.New("challenge_hints must be a string or array of strings") }; hints = strings.Join(list, "\n") } else { hints, err = getString("challenge_hints", false); if err != nil { return challengeInput{}, err } } }
	points := ""; raw, ok := fields["challenge_points"]; if !ok { return challengeInput{}, errChallengeRequired }; var number json.Number; if err := json.Unmarshal(raw, &number); err == nil { points = number.String() } else { var text string; if json.Unmarshal(raw, &text) != nil { return challengeInput{}, errors.New("challenge_points must be an integer") }; points = text }
	return challengeInput{Name:name, Description:description, Points:points, Flag:flag, Hints:hints}, nil
}
