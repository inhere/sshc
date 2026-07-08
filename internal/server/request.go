package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/gookit/rux/v2"
	"github.com/inhere/sshc/internal/core"
)

func readJSON(c *rux.Context, dst any) error {
	defer c.Req.Body.Close()
	dec := json.NewDecoder(c.Req.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return err
	}
	return nil
}

func (s *Server) rejectReadonly(c *rux.Context) bool {
	if !s.config.Readonly {
		return false
	}
	writeError(c, http.StatusForbidden, errors.New("serve is readonly"))
	return true
}

func saveCheckedConfig(config *core.Config) error {
	issues := core.CheckConfig(*config)
	if core.HasDoctorErrors(issues) {
		return fmt.Errorf("config doctor found errors: %s", doctorMessages(issues))
	}
	return core.SaveConfig(config)
}

func doctorMessages(issues []core.DoctorIssue) string {
	messages := make([]string, 0, len(issues))
	for _, issue := range issues {
		if issue.Level == core.DoctorError {
			messages = append(messages, issue.Message)
		}
	}
	return strings.Join(messages, "; ")
}

func parseTail(c *rux.Context, def int) (int, error) {
	value := strings.TrimSpace(c.Query("tail"))
	if value == "" {
		return def, nil
	}
	tail, err := strconv.Atoi(value)
	if err != nil || tail < 1 {
		return 0, fmt.Errorf("invalid tail %q", value)
	}
	return tail, nil
}

func normalizeAPIHost(host *core.Host) {
	host.Name = strings.TrimSpace(host.Name)
	host.IP = strings.TrimSpace(host.IP)
	host.AuthRef = strings.TrimSpace(host.AuthRef)
	host.User = strings.TrimSpace(host.User)
	host.KeyPath = strings.TrimSpace(host.KeyPath)
	host.Remark = strings.TrimSpace(host.Remark)
	host.Group = strings.TrimSpace(host.Group)
	host.Tags = core.NormalizeTagList(host.Tags)
	host.Jump = strings.TrimSpace(host.Jump)
	host.Backend = strings.TrimSpace(host.Backend)
	host.Via = strings.TrimSpace(host.Via)
	host.RunTemplate = strings.TrimSpace(host.RunTemplate)
	host.LoginCommand = strings.TrimSpace(host.LoginCommand)
	if host.Port == 0 {
		host.Port = core.DefaultSSHPort
	}
	if host.Group == "" {
		host.Group = core.DefaultGroup
	}
}
