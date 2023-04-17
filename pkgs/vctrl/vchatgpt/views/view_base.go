package views

import "github.com/moqsien/gvc/pkgs/vctrl/vchatgpt/vtui"

type ViewBase struct {
	Model    vtui.IModel
	ViewName string
	Enabled  bool
}

func (that *ViewBase) SetModel(m vtui.IModel) {
	that.Model = m
}

func (that *ViewBase) Name() string {
	return that.ViewName
}

func (that *ViewBase) IsEnabled() bool {
	return that.Enabled
}

func NewBase(name string) *ViewBase {
	return &ViewBase{
		ViewName: name,
	}
}
