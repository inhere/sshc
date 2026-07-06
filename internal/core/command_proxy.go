package core

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type CommandProxyRunPlan struct {
	Via            Host
	ProxiedCommand string
}

type CommandProxyLoginPlan struct {
	Via          Host
	LoginCommand string
}

var newCommandProxySSHClient = newSSHClient

func BuildRemoteRunCommand(command string, opts RunOptions) (string, error) {
	remoteCommand, err := BuildRemoteCommandWithCWD(command, opts.Env, opts.CWD)
	if err != nil {
		return "", err
	}
	remoteCommand = remoteSudoCommand(remoteCommand, opts)
	remoteCommand = remoteTimeoutCommand(remoteCommand, opts)
	return remoteCommand, nil
}

func RenderCommandProxyRun(template, finalCommand string) (string, error) {
	template = strings.TrimSpace(template)
	if template == "" {
		return "", errors.New("run_template is required for command_proxy host")
	}
	if !strings.Contains(template, CommandProxyCmdToken) {
		return "", fmt.Errorf("run_template must contain %s", CommandProxyCmdToken)
	}
	return strings.ReplaceAll(template, CommandProxyCmdToken, shellQuote(finalCommand)), nil
}

func PlanCommandProxyRun(host Host, command string, opts RunOptions) (CommandProxyRunPlan, error) {
	if !IsCommandProxyHost(host) {
		return CommandProxyRunPlan{}, fmt.Errorf("host %q is not command_proxy", HostLogName(host))
	}
	if strings.TrimSpace(opts.ScriptPath) != "" {
		return CommandProxyRunPlan{}, errors.New("--script is not supported for command_proxy hosts yet")
	}
	via, err := resolveCommandProxyVia(host)
	if err != nil {
		return CommandProxyRunPlan{}, err
	}
	finalCommand, err := BuildRemoteRunCommand(command, opts)
	if err != nil {
		return CommandProxyRunPlan{}, err
	}
	proxiedCommand, err := RenderCommandProxyRun(host.RunTemplate, finalCommand)
	if err != nil {
		return CommandProxyRunPlan{}, err
	}
	return CommandProxyRunPlan{Via: via, ProxiedCommand: proxiedCommand}, nil
}

func PlanCommandProxyLogin(host Host) (CommandProxyLoginPlan, error) {
	if !IsCommandProxyHost(host) {
		return CommandProxyLoginPlan{}, fmt.Errorf("host %q is not command_proxy", HostLogName(host))
	}
	loginCommand := strings.TrimSpace(host.LoginCommand)
	if loginCommand == "" {
		return CommandProxyLoginPlan{}, fmt.Errorf("login_command is required for command_proxy host %q", HostLogName(host))
	}
	via, err := resolveCommandProxyVia(host)
	if err != nil {
		return CommandProxyLoginPlan{}, err
	}
	return CommandProxyLoginPlan{Via: via, LoginCommand: loginCommand}, nil
}

func resolveCommandProxyVia(host Host) (Host, error) {
	config, err := LoadConfigWithSSHConfig()
	if err != nil {
		return Host{}, err
	}
	viaName := strings.TrimSpace(host.Via)
	if viaName == "" {
		return Host{}, fmt.Errorf("command_proxy host %q requires via", HostLogName(host))
	}
	viaEffective, ok, err := config.ResolveEffectiveHost(viaName, HostOverrides{})
	if err != nil {
		return Host{}, err
	}
	if !ok {
		return Host{}, fmt.Errorf("command_proxy host %q references missing via host %q", HostLogName(host), viaName)
	}
	via := viaEffective.ToHost()
	if IsCommandProxyHost(via) {
		return Host{}, fmt.Errorf("command_proxy host %q via host %q is also command_proxy", HostLogName(host), HostLogName(via))
	}
	if sameHostIdentity(host, via) || strings.TrimSpace(host.Name) == strings.TrimSpace(via.Name) {
		return Host{}, fmt.Errorf("command_proxy host %q cannot use itself as via", HostLogName(host))
	}
	return via, nil
}

func ExecuteCommandProxy(host Host, command string, opts RunOptions) ([]byte, error) {
	plan, err := PlanCommandProxyRun(host, command, opts)
	if err != nil {
		return nil, err
	}
	client, err := newCommandProxySSHClient(plan.Via)
	if err != nil {
		return nil, fmt.Errorf("connect command_proxy via host %s: %w", HostLogName(plan.Via), err)
	}
	defer client.Close()

	clientTimeout := remoteClientTimeout(opts)
	if clientTimeout <= 0 {
		return client.Run(plan.ProxiedCommand)
	}
	ctx, cancel := context.WithTimeout(context.Background(), clientTimeout)
	defer cancel()
	return client.RunContext(ctx, plan.ProxiedCommand)
}

func LoginCommandProxy(host Host, opts LoginOptions) error {
	plan, err := PlanCommandProxyLogin(host)
	if err != nil {
		return err
	}
	client, err := newCommandProxySSHClient(plan.Via)
	if err != nil {
		return fmt.Errorf("connect command_proxy via host %s: %w", HostLogName(plan.Via), err)
	}
	defer client.Close()
	return loginWithClient(client, plan.LoginCommand, opts)
}
