package stats

import (
	"fmt"

	"github.com/promptbench/backend/internal/domain"
	"github.com/promptbench/backend/internal/repo"
)

// promptFromRow 把 GetPrompt 的行映射为提示词实体，使可见性判定拿到考察目的的高风险标记。
func promptFromRow(row repo.GetPromptRow) (domain.Prompt, error) {
	visibility, ok := domain.ParseVisibility(row.Visibility)
	if !ok {
		return domain.Prompt{}, fmt.Errorf("提示词 %d 的可见性取值 %q 未登记", row.ID, row.Visibility)
	}
	sensitive, ok := domain.ParseSensitiveLevel(row.SensitiveLevel)
	if !ok {
		return domain.Prompt{}, fmt.Errorf("提示词 %d 的敏感级别取值 %q 未登记", row.ID, row.SensitiveLevel)
	}
	version, err := domain.ParseVersion(row.LatestVersionNo)
	if err != nil {
		return domain.Prompt{}, fmt.Errorf("提示词 %d: %w", row.ID, err)
	}

	out := domain.Prompt{
		ID:            row.ID,
		Title:         row.Title,
		Description:   row.Description,
		PurposeID:     row.PurposeID,
		CapabilityID:  row.CapabilityID,
		Visibility:    visibility,
		Sensitive:     sensitive,
		HighRisk:      row.IsHighRisk,
		UploaderID:    row.UploaderID,
		LatestVersion: version,
		MajorModified: row.MajorModified,
	}
	if row.DifficultyID.Valid {
		out.DifficultyID = &row.DifficultyID.Int64
	}
	if row.CurrentVersionID.Valid {
		out.CurrentVersionID = &row.CurrentVersionID.Int64
	}
	return out, nil
}
