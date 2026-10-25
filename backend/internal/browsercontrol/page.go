package browsercontrol

import (
	"context"
	"fmt"
	"sync"
)

const defaultScrollAmount = 800

type browserPage struct {
	ax        axState
	conn      *cdpConn
	opMu      sync.Mutex
	contextID int64
}

func (page *browserPage) call(ctx context.Context, input ActionInput) (ActionOutput, error) {
	switch input.Action {
	case ActionState:
		return page.semanticState(ctx)
	case ActionFind:
		return page.find(ctx, input.Text)
	case ActionScreenshot:
		return page.screenshot(ctx)
	case ActionClick:
		return page.click(ctx, input.Ref)
	case ActionHover:
		return page.hover(ctx, input.Ref)
	case ActionDrag:
		return page.drag(ctx, input.Ref, input.Text)
	case ActionFormInput:
		return page.formInput(ctx, input.Ref, input.Value)
	case ActionPress:
		return page.press(ctx, input.Ref, input.Key)
	case ActionScroll:
		return page.scroll(ctx, input.Ref, input.Text, input.Amount)
	case ActionWait:
		return page.wait(ctx, input.Ref, input.Text, input.Amount)
	}
	return ActionOutput{}, UnsupportedActionError{Action: input.Action}
}

func (p *browserPage) close() {
	_ = p.conn.Close()
}

func (p *browserPage) info(ctx context.Context) (string, error) {
	var info struct {
		Title string `json:"title"`
		URL   string `json:"url"`
		Ready string `json:"ready"`
	}
	if err := p.eval(ctx, `({title: document.title, url: location.href, ready: document.readyState})`, &info); err != nil {
		return "", err
	}
	return fmt.Sprintf(
		"title: %s\nurl: %s\nready: %s",
		shortenText(info.Title, 500),
		shortenText(info.URL, 2000),
		shortenText(info.Ready, 50),
	), nil
}
