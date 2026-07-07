package main

import (
	"context"
	"fmt"
	"os"

	"github.com/shikanon/orag/examples/scenarios/internal/demo"
)

func main() {
	scenario := demo.Scenario{
		ID:           "agent-developer",
		Title:        "Agent Developer",
		Role:         "IDE, CLI, and MCP agent developers",
		BusinessGoal: "Expose ORAG knowledge, verification, and diagnostics as tool-style responses with trace evidence.",
		UserQuestion: "How should an agent developer use ORAG to expose bounded verification and read-only diagnostics with trace evidence?",
		DemoDataPaths: []string{
			"examples/scenarios/agent-developer/demo-data.md",
			"demo-data.md",
		},
		SourceURI: "example://scenarios/agent-developer/demo-data.md",
		Profile:   "agent-tooling",
		TopK:      4,
		Dimensions: []demo.Dimension{
			{Name: "使用方", Value: "Agent 开发者、IDE 插件开发者、自动化平台团队"},
			{Name: "业务问题", Value: "Agent 需要结构化调用 ORAG，而不是拼接临时 shell 或发散式修复"},
			{Name: "输入数据", Value: "MCP 工具契约、Skill prompt、授权环境变量、验证边界"},
			{Name: "ORAG能力", Value: "MCP tool discovery、Ralph Loop、self-check、trace evidence、Skill packaging"},
			{Name: "成功标准", Value: "工具输出包含 verdict、artifacts、trace_id，并遵守只读/审批边界"},
		},
		ExpectedSignals: []string{
			"answer references bounded verification and read-only diagnostics",
			"trace_id is printed as evidence for the agent response",
			"recommended_next_steps mention MCP and Skill integration checks",
		},
		RecommendedSteps: []string{
			"Copy examples/mcp/stdio-client-config.json into the target MCP client.",
			"Run make agent-sync-check before publishing generated tool or Skill artifacts.",
			"Keep write operations behind explicit approval and report blocked when evidence is missing.",
		},
	}
	if err := demo.Run(context.Background(), os.Stdout, scenario); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
