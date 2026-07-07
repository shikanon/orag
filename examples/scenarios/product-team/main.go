package main

import (
	"context"
	"fmt"
	"os"

	"github.com/shikanon/orag/examples/scenarios/internal/demo"
)

func main() {
	scenario := demo.Scenario{
		ID:           "product-team",
		Title:        "Product Team",
		Role:         "Product managers and AI feature owners",
		BusinessGoal: "Review whether a knowledge assistant is ready to launch and what evidence supports the decision.",
		UserQuestion: "Is the onboarding assistant ready to launch, and what evidence should product review?",
		DemoDataPaths: []string{
			"examples/scenarios/product-team/demo-data.md",
			"demo-data.md",
		},
		SourceURI: "example://scenarios/product-team/demo-data.md",
		Profile:   "launch-review",
		TopK:      3,
		Dimensions: []demo.Dimension{
			{Name: "使用方", Value: "产品经理、AI 功能 owner、质量评审负责人"},
			{Name: "业务问题", Value: "上线前需要判断回答质量、引用可信度和检索配置是否达标"},
			{Name: "输入数据", Value: "上线验收标准、代表性用户问题、评估集、优化候选配置"},
			{Name: "ORAG能力", Value: "问答、引用、trace、评估指标、优化候选"},
			{Name: "成功标准", Value: "答案可解释，质量信号可复查，下一步上线/调优决策明确"},
		},
		ExpectedSignals: []string{
			"answer describes launch-readiness evidence",
			"citations > 0 for product review",
			"recommended_next_steps include evaluation and optimization follow-up",
		},
		RecommendedSteps: []string{
			"Replace demo-data.md with product launch criteria and real representative questions.",
			"Use service evaluation scripts to turn qualitative review into repeatable metrics.",
			"Document accepted profile and top-k settings alongside launch notes.",
		},
	}
	if err := demo.Run(context.Background(), os.Stdout, scenario); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
