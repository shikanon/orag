package offlineknowledge

import "context"

type DisabledItemValidator struct{}

func (DisabledItemValidator) ValidateItem(context.Context, string, string, OptimizationItem) error {
	return ErrValidatorDisabled
}

type DisabledRegressionRunner struct{}

func (DisabledRegressionRunner) RunRegression(context.Context, RegressionRequest) (RegressionResult, error) {
	return RegressionResult{}, ErrRegressionDisabled
}

type UnavailableRegressionRunner struct{}

func (UnavailableRegressionRunner) RunRegression(context.Context, RegressionRequest) (RegressionResult, error) {
	return RegressionResult{}, ErrRegressionUnavailable
}
