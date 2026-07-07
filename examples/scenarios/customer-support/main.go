package main

import (
	"context"
	"fmt"
	"os"

	"github.com/shikanon/orag/examples/scenarios/internal/demo"
)

func main() {
	scenario := demo.Scenario{
		ID:           "customer-support",
		Title:        "Customer Support",
		Role:         "Customer support and pre-sales teams",
		BusinessGoal: "Answer customer questions from product knowledge with citations and trace evidence for escalation.",
		UserQuestion: "A customer asks why ORAG answers must include citations and what trace ID should be shared for escalation. How should support reply?",
		DemoDataPaths: []string{
			"examples/scenarios/customer-support/demo-data.md",
			"demo-data.md",
		},
		SourceURI: "example://scenarios/customer-support/demo-data.md",
		Profile:   "support-console",
		TopK:      3,
		Dimensions: []demo.Dimension{
			{Name: "使用方", Value: "客服、售前、服务台人员"},
			{Name: "业务问题", Value: "客户提问需要基于产品资料快速给出可信答案"},
			{Name: "输入数据", Value: "FAQ、支持政策、故障排查说明、升级规则"},
			{Name: "ORAG能力", Value: "文档入库、检索问答、引用、trace 查询"},
			{Name: "成功标准", Value: "答案可直接回复客户，引用可核验，trace_id 可交给研发排查"},
		},
		ExpectedSignals: []string{
			"answer contains support guidance grounded in demo-data.md",
			"citations > 0",
			"trace_id is stable for escalation evidence",
		},
		RecommendedSteps: []string{
			"Replace demo-data.md with real support policy and product FAQ content.",
			"Pipe the printed trace_id into the service trace lookup flow when a customer escalates.",
			"Add tenant or product metadata before onboarding multiple support queues.",
		},
	}
	if err := demo.Run(context.Background(), os.Stdout, scenario); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
