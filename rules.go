package graphql

import (
	"fmt"
	"github.com/graphql-go/graphql/gqlerrors"
	"github.com/graphql-go/graphql/language/ast"
	"github.com/graphql-go/graphql/language/kinds"
	"github.com/graphql-go/graphql/language/printer"
	"github.com/graphql-go/graphql/language/visitor"
)

/**
 * SpecifiedRules set includes all validation rules defined by the GraphQL spec.
 */
var SpecifiedRules = []ValidationRuleFn{
	ArgumentsOfCorrectTypeRule,
	DefaultValuesOfCorrectTypeRule,
	FieldsOnCorrectTypeRule,
	FragmentsOnCompositeTypesRule,
	KnownArgumentNamesRule,
	KnownDirectivesRule,
	KnownFragmentNamesRule,
	KnownTypeNamesRule,
}

type ValidationRuleInstance struct {
	VisitorOpts          *visitor.VisitorOptions
	VisitSpreadFragments bool
}
type ValidationRuleFn func(context *ValidationContext) *ValidationRuleInstance

func newValidationRuleError(message string, nodes []ast.Node) (string, error) {
	return visitor.ActionNoChange, gqlerrors.NewError(
		message,
		nodes,
		"",
		nil,
		[]int{},
	)
}

/**
 * ArgumentsOfCorrectTypeRule
 * Argument values of correct type
 *
 * A GraphQL document is only valid if all field argument literal values are
 * of the type expected by their position.
 */
func ArgumentsOfCorrectTypeRule(context *ValidationContext) *ValidationRuleInstance {
	visitorOpts := &visitor.VisitorOptions{
		KindFuncMap: map[string]visitor.NamedVisitFuncs{
			kinds.Argument: visitor.NamedVisitFuncs{
				Kind: func(p visitor.VisitFuncParams) (string, interface{}) {
					var action = visitor.ActionNoChange
					var result interface{}
					if argAST, ok := p.Node.(*ast.Argument); ok {
						value := argAST.Value
						argDef := context.GetArgument()
						if argDef != nil && !isValidLiteralValue(argDef.Type, value) {
							argNameValue := ""
							if argAST.Name != nil {
								argNameValue = argAST.Name.Value
							}
							return newValidationRuleError(
								fmt.Sprintf(`Argument "%v" expected type "%v" but got: %v.`,
									argNameValue, argDef.Type, printer.Print(value)),
								[]ast.Node{value},
							)
						}
					}
					return action, result
				},
			},
		},
	}
	return &ValidationRuleInstance{
		VisitorOpts: visitorOpts,
	}
}

/**
 * DefaultValuesOfCorrectTypeRule
 * Variable default values of correct type
 *
 * A GraphQL document is only valid if all variable default values are of the
 * type expected by their definition.
 */
func DefaultValuesOfCorrectTypeRule(context *ValidationContext) *ValidationRuleInstance {
	visitorOpts := &visitor.VisitorOptions{
		KindFuncMap: map[string]visitor.NamedVisitFuncs{
			kinds.VariableDefinition: visitor.NamedVisitFuncs{
				Kind: func(p visitor.VisitFuncParams) (string, interface{}) {
					var action = visitor.ActionNoChange
					var result interface{}
					if varDefAST, ok := p.Node.(*ast.VariableDefinition); ok {
						name := ""
						if varDefAST.Variable != nil && varDefAST.Variable.Name != nil {
							name = varDefAST.Variable.Name.Value
						}
						defaultValue := varDefAST.DefaultValue
						ttype := context.GetInputType()

						if ttype, ok := ttype.(*NonNull); ok && defaultValue != nil {
							return newValidationRuleError(
								fmt.Sprintf(`Variable "$%v" of type "%v" is required and will not use the default value. Perhaps you meant to use type "%v".`,
									name, ttype, ttype.OfType),
								[]ast.Node{defaultValue},
							)
						}
						if ttype != nil && defaultValue != nil && !isValidLiteralValue(ttype, defaultValue) {
							return newValidationRuleError(
								fmt.Sprintf(`Variable "$%v" of type "%v" has invalid default value: %v.`,
									name, ttype, printer.Print(defaultValue)),
								[]ast.Node{defaultValue},
							)
						}
					}
					return action, result
				},
			},
		},
	}
	return &ValidationRuleInstance{
		VisitorOpts: visitorOpts,
	}
}

/**
 * FieldsOnCorrectTypeRule
 * Fields on correct type
 *
 * A GraphQL document is only valid if all fields selected are defined by the
 * parent type, or are an allowed meta field such as __typenamme
 */
func FieldsOnCorrectTypeRule(context *ValidationContext) *ValidationRuleInstance {
	visitorOpts := &visitor.VisitorOptions{
		KindFuncMap: map[string]visitor.NamedVisitFuncs{
			kinds.Field: visitor.NamedVisitFuncs{
				Kind: func(p visitor.VisitFuncParams) (string, interface{}) {
					var action = visitor.ActionNoChange
					var result interface{}
					if node, ok := p.Node.(*ast.Field); ok {
						ttype := context.GetParentType()

						if ttype != nil {
							fieldDef := context.GetFieldDef()
							if fieldDef == nil {
								nodeName := ""
								if node.Name != nil {
									nodeName = node.Name.Value
								}
								return newValidationRuleError(
									fmt.Sprintf(`Cannot query field "%v" on "%v".`,
										nodeName, ttype.GetName()),
									[]ast.Node{node},
								)
							}
						}
					}
					return action, result
				},
			},
		},
	}
	return &ValidationRuleInstance{
		VisitorOpts: visitorOpts,
	}
}

/**
 * FragmentsOnCompositeTypesRule
 * Fragments on composite type
 *
 * Fragments use a type condition to determine if they apply, since fragments
 * can only be spread into a composite type (object, interface, or union), the
 * type condition must also be a composite type.
 */
func FragmentsOnCompositeTypesRule(context *ValidationContext) *ValidationRuleInstance {
	visitorOpts := &visitor.VisitorOptions{
		KindFuncMap: map[string]visitor.NamedVisitFuncs{
			kinds.InlineFragment: visitor.NamedVisitFuncs{
				Kind: func(p visitor.VisitFuncParams) (string, interface{}) {
					if node, ok := p.Node.(*ast.InlineFragment); ok {
						ttype := context.GetType()
						if ttype != nil && !IsCompositeType(ttype) {
							return newValidationRuleError(
								fmt.Sprintf(`Fragment cannot condition on non composite type "%v".`, ttype),
								[]ast.Node{node.TypeCondition},
							)
						}
					}
					return visitor.ActionNoChange, nil
				},
			},
			kinds.FragmentDefinition: visitor.NamedVisitFuncs{
				Kind: func(p visitor.VisitFuncParams) (string, interface{}) {
					if node, ok := p.Node.(*ast.FragmentDefinition); ok {
						ttype := context.GetType()
						if ttype != nil && !IsCompositeType(ttype) {
							nodeName := ""
							if node.Name != nil {
								nodeName = node.Name.Value
							}
							return newValidationRuleError(
								fmt.Sprintf(`Fragment "%v" cannot condition on non composite type "%v".`, nodeName, printer.Print(node.TypeCondition)),
								[]ast.Node{node.TypeCondition},
							)
						}
					}
					return visitor.ActionNoChange, nil
				},
			},
		},
	}
	return &ValidationRuleInstance{
		VisitorOpts: visitorOpts,
	}
}

/**
 * KnownArgumentNamesRule
 * Known argument names
 *
 * A GraphQL field is only valid if all supplied arguments are defined by
 * that field.
 */
func KnownArgumentNamesRule(context *ValidationContext) *ValidationRuleInstance {
	visitorOpts := &visitor.VisitorOptions{
		KindFuncMap: map[string]visitor.NamedVisitFuncs{
			kinds.Argument: visitor.NamedVisitFuncs{
				Kind: func(p visitor.VisitFuncParams) (string, interface{}) {
					var action = visitor.ActionNoChange
					var result interface{}
					if node, ok := p.Node.(*ast.Argument); ok {
						var argumentOf ast.Node
						if len(p.Ancestors) > 0 {
							argumentOf = p.Ancestors[len(p.Ancestors)-1]
						}
						if argumentOf == nil {
							return action, result
						}
						if argumentOf.GetKind() == "Field" {
							fieldDef := context.GetFieldDef()
							if fieldDef == nil {
								return action, result
							}
							nodeName := ""
							if node.Name != nil {
								nodeName = node.Name.Value
							}
							var fieldArgDef *Argument
							for _, arg := range fieldDef.Args {
								if arg.Name == nodeName {
									fieldArgDef = arg
								}
							}
							if fieldArgDef == nil {
								parentType := context.GetParentType()
								parentTypeName := ""
								if parentType != nil {
									parentTypeName = parentType.GetName()
								}
								return newValidationRuleError(
									fmt.Sprintf(`Unknown argument "%v" on field "%v" of type "%v".`, nodeName, fieldDef.Name, parentTypeName),
									[]ast.Node{node},
								)
							}
						} else if argumentOf.GetKind() == "Directive" {
							directive := context.GetDirective()
							if directive == nil {
								return action, result
							}
							nodeName := ""
							if node.Name != nil {
								nodeName = node.Name.Value
							}
							var directiveArgDef *Argument
							for _, arg := range directive.Args {
								if arg.Name == nodeName {
									directiveArgDef = arg
								}
							}
							if directiveArgDef == nil {
								return newValidationRuleError(
									fmt.Sprintf(`Unknown argument "%v" on directive "@%v".`, nodeName, directive.Name),
									[]ast.Node{node},
								)
							}
						}

					}
					return action, result
				},
			},
		},
	}
	return &ValidationRuleInstance{
		VisitorOpts: visitorOpts,
	}
}

/**
 * Known directives
 *
 * A GraphQL document is only valid if all `@directives` are known by the
 * schema and legally positioned.
 */
func KnownDirectivesRule(context *ValidationContext) *ValidationRuleInstance {
	visitorOpts := &visitor.VisitorOptions{
		KindFuncMap: map[string]visitor.NamedVisitFuncs{
			kinds.Directive: visitor.NamedVisitFuncs{
				Kind: func(p visitor.VisitFuncParams) (string, interface{}) {
					var action = visitor.ActionNoChange
					var result interface{}
					if node, ok := p.Node.(*ast.Directive); ok {

						nodeName := ""
						if node.Name != nil {
							nodeName = node.Name.Value
						}

						var directiveDef *Directive
						for _, def := range context.GetSchema().GetDirectives() {
							if def.Name == nodeName {
								directiveDef = def
							}
						}
						if directiveDef == nil {
							return newValidationRuleError(
								fmt.Sprintf(`Unknown directive "%v".`, nodeName),
								[]ast.Node{node},
							)
						}

						var appliedTo ast.Node
						if len(p.Ancestors) > 0 {
							appliedTo = p.Ancestors[len(p.Ancestors)-1]
						}
						if appliedTo == nil {
							return action, result
						}

						if appliedTo.GetKind() == kinds.OperationDefinition && directiveDef.OnOperation == false {
							return newValidationRuleError(
								fmt.Sprintf(`Directive "%v" may not be used on "%v".`, nodeName, "operation"),
								[]ast.Node{node},
							)
						}
						if appliedTo.GetKind() == kinds.Field && directiveDef.OnField == false {
							return newValidationRuleError(
								fmt.Sprintf(`Directive "%v" may not be used on "%v".`, nodeName, "field"),
								[]ast.Node{node},
							)
						}
						if (appliedTo.GetKind() == kinds.FragmentSpread ||
							appliedTo.GetKind() == kinds.InlineFragment ||
							appliedTo.GetKind() == kinds.FragmentDefinition) && directiveDef.OnFragment == false {
							return newValidationRuleError(
								fmt.Sprintf(`Directive "%v" may not be used on "%v".`, nodeName, "fragment"),
								[]ast.Node{node},
							)
						}

					}
					return action, result
				},
			},
		},
	}
	return &ValidationRuleInstance{
		VisitorOpts: visitorOpts,
	}
}

/**
 * KnownFragmentNamesRule
 * Known fragment names
 *
 * A GraphQL document is only valid if all `...Fragment` fragment spreads refer
 * to fragments defined in the same document.
 */
func KnownFragmentNamesRule(context *ValidationContext) *ValidationRuleInstance {
	visitorOpts := &visitor.VisitorOptions{
		KindFuncMap: map[string]visitor.NamedVisitFuncs{
			kinds.FragmentSpread: visitor.NamedVisitFuncs{
				Kind: func(p visitor.VisitFuncParams) (string, interface{}) {
					var action = visitor.ActionNoChange
					var result interface{}
					if node, ok := p.Node.(*ast.FragmentSpread); ok {

						fragmentName := ""
						if node.Name != nil {
							fragmentName = node.Name.Value
						}

						fragment := context.GetFragment(fragmentName)
						if fragment == nil {
							return newValidationRuleError(
								fmt.Sprintf(`Unknown fragment "%v".`, fragmentName),
								[]ast.Node{node.Name},
							)
						}
					}
					return action, result
				},
			},
		},
	}
	return &ValidationRuleInstance{
		VisitorOpts: visitorOpts,
	}
}

/**
 * KnownTypeNamesRule
 * Known type names
 *
 * A GraphQL document is only valid if referenced types (specifically
 * variable definitions and fragment conditions) are defined by the type schema.
 */
func KnownTypeNamesRule(context *ValidationContext) *ValidationRuleInstance {
	visitorOpts := &visitor.VisitorOptions{
		KindFuncMap: map[string]visitor.NamedVisitFuncs{
			kinds.Named: visitor.NamedVisitFuncs{
				Kind: func(p visitor.VisitFuncParams) (string, interface{}) {
					if node, ok := p.Node.(*ast.Named); ok {
						typeNameValue := ""
						typeName := node.Name
						if typeName != nil {
							typeNameValue = typeName.Value
						}
						ttype := context.GetSchema().GetType(typeNameValue)
						if ttype == nil {
							return newValidationRuleError(
								fmt.Sprintf(`Unknown type "%v".`, typeNameValue),
								[]ast.Node{node},
							)
						}
					}
					return visitor.ActionNoChange, nil
				},
			},
		},
	}
	return &ValidationRuleInstance{
		VisitorOpts: visitorOpts,
	}
}

/**
 * Utility for validators which determines if a value literal AST is valid given
 * an input type.
 *
 * Note that this only validates literal values, variables are assumed to
 * provide values of the correct type.
 */
func isValidLiteralValue(ttype Input, valueAST ast.Value) bool {
	// A value must be provided if the type is non-null.
	if ttype, ok := ttype.(*NonNull); ok {
		if valueAST == nil {
			return false
		}
		ofType, _ := ttype.OfType.(Input)
		return isValidLiteralValue(ofType, valueAST)
	}

	if valueAST == nil {
		return true
	}

	// This function only tests literals, and assumes variables will provide
	// values of the correct type.
	if valueAST.GetKind() == kinds.Variable {
		return true
	}

	// Lists accept a non-list value as a list of one.
	if ttype, ok := ttype.(*List); ok {
		itemType, _ := ttype.OfType.(Input)
		if valueAST, ok := valueAST.(*ast.ListValue); ok {
			for _, value := range valueAST.Values {
				if isValidLiteralValue(itemType, value) == false {
					return false
				}
			}
			return true
		}
		return isValidLiteralValue(itemType, valueAST)

	}

	// Input objects check each defined field and look for undefined fields.
	if ttype, ok := ttype.(*InputObject); ok {
		valueAST, ok := valueAST.(*ast.ObjectValue)
		if !ok {
			return false
		}
		fields := ttype.GetFields()

		// Ensure every provided field is defined.
		// Ensure every defined field is valid.
		fieldASTs := valueAST.Fields
		fieldASTMap := map[string]*ast.ObjectField{}
		for _, fieldAST := range fieldASTs {
			fieldASTName := ""
			if fieldAST.Name != nil {
				fieldASTName = fieldAST.Name.Value
			}

			fieldASTMap[fieldASTName] = fieldAST

			// check if field is defined
			field, ok := fields[fieldASTName]
			if !ok || field == nil {
				return false
			}
		}
		for fieldName, field := range fields {
			fieldAST, _ := fieldASTMap[fieldName]
			var fieldASTValue ast.Value
			if fieldAST != nil {
				fieldASTValue = fieldAST.Value
			}
			if !isValidLiteralValue(field.Type, fieldASTValue) {
				return false
			}
		}
		return true
	}

	if ttype, ok := ttype.(*Scalar); ok {
		return !isNullish(ttype.ParseLiteral(valueAST))
	}
	if ttype, ok := ttype.(*Enum); ok {
		return !isNullish(ttype.ParseLiteral(valueAST))
	}

	// Must be input type (not scalar or enum)
	// Silently fail, instead of panic()
	return false
}
