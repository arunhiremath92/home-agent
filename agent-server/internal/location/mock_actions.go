package location

import (
	"strings"

	"github.com/aruhiremath92/agent-server/internal/tasks"
)

type MockActionEstimator struct{}

func (MockActionEstimator) Estimate(pending []tasks.Task, atHome bool) []TaskAction {
	if !atHome {
		return []TaskAction{}
	}
	actions := make([]TaskAction, 0)
	for _, task := range pending {
		text := strings.ToLower(task.Title + " " + task.Description)
		actionType, minutes := "", 0
		switch {
		case strings.Contains(text, "roomba"), strings.Contains(text, "robot vacuum"):
			actionType, minutes = "start_robot_vacuum", 45
		case strings.Contains(text, "clean"), strings.Contains(text, "vacuum"):
			actionType, minutes = "cleaning", 30
		case strings.Contains(text, "laundry"), strings.Contains(text, "dish"):
			actionType, minutes = "household_chore", 25
		}
		if actionType == "" {
			continue
		}
		actions = append(actions, TaskAction{
			TaskID:          task.ID,
			TaskTitle:       task.Title,
			ActionType:      actionType,
			EstimatedMinute: minutes,
			Source:          "mock",
		})
	}
	return actions
}
