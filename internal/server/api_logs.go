package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/gookit/rux/v2"
	"github.com/inhere/sshc/internal/core"
)

type logOutputResponse struct {
	TaskID string `json:"task_id"`
	Output string `json:"output"`
}

func (s *Server) handleLogsList(c *rux.Context) {
	tail, err := parseTail(c, 50)
	if err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	lines, err := core.ReadRunLogsSelected(c.Query("target"), c.Query("match"), tail, c.Query("lines"))
	if err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	records := parseLogLines(lines)
	writeOK(c, records)
}

func (s *Server) handleLogsShow(c *rux.Context) {
	taskID := c.Param("task_id")
	record, ok, err := findLogRecord(taskID, c.Query("target"))
	if err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	if !ok {
		writeError(c, http.StatusNotFound, errTaskNotFound(taskID))
		return
	}
	writeOK(c, record)
}

func (s *Server) handleLogOutput(c *rux.Context) {
	taskID := c.Param("task_id")
	data, err := core.ReadRunLogOutputByID(c.Query("target"), taskID)
	if err != nil {
		writeError(c, http.StatusNotFound, err)
		return
	}
	lines := splitOutputLines(string(data))
	tail, err := parseTail(c, 0)
	if err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	}
	if selected, err := core.SelectLogLines(lines, c.Query("lines"), tail); err != nil {
		writeError(c, http.StatusBadRequest, err)
		return
	} else if c.Query("lines") != "" || c.Query("tail") != "" {
		data = []byte(strings.Join(selected, "\n"))
		if len(selected) > 0 {
			data = append(data, '\n')
		}
	}
	writeOK(c, logOutputResponse{TaskID: taskID, Output: string(data)})
}

func findLogRecord(taskID, target string) (map[string]any, bool, error) {
	lines, err := core.ReadRunLogsSelected(target, "", 1<<30, "")
	if err != nil {
		return nil, false, err
	}
	for i := len(lines) - 1; i >= 0; i-- {
		var record map[string]any
		if err := json.Unmarshal([]byte(lines[i]), &record); err != nil {
			continue
		}
		if record["task_id"] == taskID {
			return record, true, nil
		}
	}
	return nil, false, nil
}

func parseLogLines(lines []string) []map[string]any {
	records := make([]map[string]any, 0, len(lines))
	for _, line := range lines {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err == nil {
			records = append(records, record)
		}
	}
	return records
}

func splitOutputLines(output string) []string {
	output = strings.TrimSuffix(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	if output == "" {
		return nil
	}
	return strings.Split(output, "\n")
}

func errTaskNotFound(taskID string) error {
	return &taskNotFoundError{taskID: taskID}
}

type taskNotFoundError struct {
	taskID string
}

func (e *taskNotFoundError) Error() string {
	return "task " + e.taskID + " not found"
}
