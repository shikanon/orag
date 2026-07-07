package main

import (
	"context"
	"fmt"
	"os"

	"github.com/shikanon/orag/examples/scenarios/internal/demo"
)

func main() {
	scenario := demo.Scenario{
		ID:           "engineering-runbook",
		Title:        "Engineering Runbook",
		Role:         "Engineering and SRE teams",
		BusinessGoal: "Search runbooks and incident notes during debugging, then preserve trace evidence for follow-up.",
		UserQuestion: "The query API is slower after a deploy. Which runbook checks should I run first, and what ORAG trace evidence should I attach?",
		DemoDataPaths: []string{
			"examples/scenarios/engineering-runbook/demo-data.md",
			"demo-data.md",
		},
		SourceURI: "example://scenarios/engineering-runbook/demo-data.md",
		Profile:   "incident-triage",
		TopK:      3,
		Dimensions: []demo.Dimension{
			{Name: "使用方", Value: "研发、SRE、值班工程师"},
			{Name: "业务问题", Value: "故障期间需要快速定位排查步骤和升级证据"},
			{Name: "输入数据", Value: "runbook、事故复盘、架构说明、API 排障文档"},
			{Name: "ORAG能力", Value: "知识检索、答案生成、trace 元数据、引用来源"},
			{Name: "成功标准", Value: "输出可执行排障步骤，并带有 trace_id、引用和慢节点摘要"},
		},
		ExpectedSignals: []string{
			"answer mentions readiness, trace, and escalation evidence",
			"trace_summary includes retrieve and generate_answer spans",
			"first citation points back to engineering-runbook demo data",
		},
		RecommendedSteps: []string{
			"Replace demo-data.md with team runbooks and recent incident notes.",
			"Attach trace_id, query text, and cited runbook section to the incident ticket.",
			"Use evaluation and optimization demos after the incident to tune retrieval settings safely.",
		},
	}
	if err := demo.Run(context.Background(), os.Stdout, scenario); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
