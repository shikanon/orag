package main

import (
	"context"
	"fmt"
	"os"

	"github.com/shikanon/orag/examples/scenarios/internal/demo"
)

func main() {
	scenario := demo.Scenario{
		ID:           "platform-team",
		Title:        "Platform Team",
		Role:         "Internal AI platform and infrastructure teams",
		BusinessGoal: "Offer ORAG as a shared RAG service with onboarding, quality, traceability, and agent integration evidence.",
		UserQuestion: "Which ORAG checks should a platform team run before offering RAG as a shared service?",
		DemoDataPaths: []string{
			"examples/scenarios/platform-team/demo-data.md",
			"demo-data.md",
		},
		SourceURI: "example://scenarios/platform-team/demo-data.md",
		Profile:   "platform-readiness",
		TopK:      4,
		Dimensions: []demo.Dimension{
			{Name: "使用方", Value: "AI 平台团队、基础架构团队、内部能力提供方"},
			{Name: "业务问题", Value: "多业务接入前需要一套可复用的 RAG 服务验收路径"},
			{Name: "输入数据", Value: "平台接入手册、服务 readiness 规则、质量门禁、agent 资产规范"},
			{Name: "ORAG能力", Value: "认证、知识库、入库、查询、trace、评估、优化、MCP/Skill 资产"},
			{Name: "成功标准", Value: "服务路径可 smoke，质量可度量，工具资产与 OpenAPI 同步"},
		},
		ExpectedSignals: []string{
			"answer lists service readiness checks",
			"citations connect readiness guidance to demo-data.md",
			"trace_id can be used as platform smoke evidence",
		},
		RecommendedSteps: []string{
			"Use this Go demo for local onboarding education before running service-mode curl examples.",
			"Run make agent-sync-check in CI to protect MCP and Skill contracts.",
			"Publish role-specific README links to application teams as the first integration path.",
		},
	}
	if err := demo.Run(context.Background(), os.Stdout, scenario); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
