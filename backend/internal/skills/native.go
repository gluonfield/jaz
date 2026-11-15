package skills

import (
	"context"
	"slices"
)

type nativeSkillsKey struct{}

func WithNativeSkills(ctx context.Context, native []Skill) context.Context {
	return context.WithValue(ctx, nativeSkillsKey{}, native)
}

func (c Catalog) PromptForContext(ctx context.Context) string {
	native, _ := ctx.Value(nativeSkillsKey{}).([]Skill)
	if len(native) == 0 {
		return c.Prompt()
	}
	c.Skills = slices.DeleteFunc(slices.Clone(c.Skills), func(skill Skill) bool {
		return slices.ContainsFunc(native, func(owned Skill) bool {
			return skill.Name == owned.Name && skill.Path == owned.Path
		})
	})
	return c.Prompt()
}
