package rule

import (
	"context"
	"fmt"

	"github.com/UnitVectorY-Labs/ghwhenthen/internal/config"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/event"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/expr"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/resolve"
	"github.com/UnitVectorY-Labs/ghwhenthen/internal/step"
)

// CompiledRule is a rule with a pre-parsed expression.
type CompiledRule struct {
	Config     *config.Rule
	Expression expr.Expression
}

// Engine evaluates rules against events and executes actions.
type Engine struct {
	Rules     []CompiledRule
	Endpoint  string
	Token     string
	Constants map[string]interface{}
}

// NewEngine compiles rules and creates the engine.
// Returns error if any when expression fails to parse.
func NewEngine(rules []config.Rule, constants map[string]interface{}, endpoint string, token string) (*Engine, error) {
	compiled := make([]CompiledRule, 0, len(rules))
	for i := range rules {
		expression, err := expr.Parse(rules[i].When)
		if err != nil {
			return nil, fmt.Errorf("compiling rule %q: %w", rules[i].Name, err)
		}
		compiled = append(compiled, CompiledRule{
			Config:     &rules[i],
			Expression: expression,
		})
	}
	return &Engine{
		Rules:     compiled,
		Endpoint:  endpoint,
		Token:     token,
		Constants: constants,
	}, nil
}

// ProcessEvent evaluates rules against the event and executes the first match.
// Returns:
//   - matched (bool): whether a rule matched
//   - ruleName (string): the name of the matched rule (empty if no match)
//   - err (error): processing error if step execution failed
func (eng *Engine) ProcessEvent(ctx context.Context, evt *event.Event) (matched bool, ruleName string, err error) {
	for _, rule := range eng.Rules {
		if !rule.Config.Enabled {
			continue
		}
		if !rule.Expression.Evaluate(evt) {
			continue
		}

		resolveCtx := &resolve.ResolveContext{
			Event:     evt,
			Constants: eng.Constants,
			Steps:     make(map[string]map[string]interface{}),
		}

		for _, s := range rule.Config.Then {
			executor, err := step.GetExecutor(s.Type, eng.Endpoint, eng.Token)
			if err != nil {
				return true, rule.Config.Name, fmt.Errorf("step %q: %w", s.Name, err)
			}

			outputs, err := executor.Execute(ctx, &s, resolveCtx)
			if err != nil {
				return true, rule.Config.Name, fmt.Errorf("step %q: %w", s.Name, err)
			}

			resolveCtx.Steps[s.Name] = outputs
		}

		return true, rule.Config.Name, nil
	}

	return false, "", nil
}
