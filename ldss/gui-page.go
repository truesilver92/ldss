package main

import (
	"github.com/andlabs/ui"
	"ldss/lib"
)

type guiPage struct {
	app                                                 *gui
	item                                                lib.Item
	lang                                                *lib.Language
	box, toolbar, contents                              *ui.Box
	address                                             *ui.Entry
	title, status                                       *ui.Label
	btnUp, btnNext, btnPrevious, btnNewTab, btnCloseTab *ui.Button
	childMap                                            map[uintptr]string
}

func newGuiPage() *guiPage {
	var err error
	p := &guiPage{}

	p.childMap = make(map[uintptr]string)

	p.box = ui.NewVerticalBox()
	p.toolbar = ui.NewHorizontalBox()
	p.contents = ui.NewVerticalBox()

	p.btnUp = ui.NewButton("Up")
	p.btnUp.OnClicked(func(btn *ui.Button) {
		p.SetItem(p.item.Parent(), true)
	})
	p.btnNext = ui.NewButton("Next")
	p.btnNext.OnClicked(func(btn *ui.Button) {
		p.SetItem(p.item.Next(), true)
	})
	p.btnPrevious = ui.NewButton("Previous")
	p.btnPrevious.OnClicked(func(btn *ui.Button) {
		p.SetItem(p.item.Previous(), true)
	})
	p.btnNewTab = ui.NewButton("+")
	p.btnCloseTab = ui.NewButton("-")

	p.lang, err = lib.DefaultLanguage()

	if err != nil {
		panic(err)
	}

	p.title = ui.NewLabel("LDS Scriptures")
	p.status = ui.NewLabel("")
	p.address = ui.NewEntry()

	p.address.OnChanged(p.onPathChanged)

	p.toolbar.Append(p.btnPrevious, false)
	p.toolbar.Append(p.btnUp, false)
	p.toolbar.Append(p.address, true)
	p.toolbar.Append(p.btnNext, false)
	p.toolbar.Append(p.btnNewTab, false)
	p.toolbar.Append(p.btnCloseTab, false)
	p.box.Append(p.title, false)
	p.box.Append(p.toolbar, false)
	p.box.Append(p.contents, true)
	p.box.Append(p.status, false)

	return p
}

func (p *guiPage) Lookup(s string) {
	p.handleMessages(lib.LookupPath(p.lang, s), true)
}

func toggleBtn(btn *ui.Button, item interface{}) {
	if item == nil {
		btn.Disable()
	} else {
		btn.Enable()
	}
}

func (p *guiPage) SetItem(item lib.Item, setText bool) {
	if p.item != nil {
		p.contents.Delete(0)
	}
	p.childMap = make(map[uintptr]string)
	if item == nil {
		p.title.SetText("")
		p.btnUp.Disable()
		p.btnNext.Disable()
		p.btnPrevious.Disable()
	} else {
		toggleBtn(p.btnUp, item.Parent())
		toggleBtn(p.btnNext, item.Next())
		toggleBtn(p.btnPrevious, item.Previous())
		p.title.SetText(item.String())
		if setText {
			p.address.SetText(item.Path())
		}
		children, err := item.Children()
		if err != nil {
			p.ShowError(err)
			return
		}
		colsGrp := ui.NewHorizontalBox()
		cols := []*ui.Box{
			ui.NewVerticalBox(),
			ui.NewVerticalBox(),
			ui.NewVerticalBox(),
		}
		for i, c := range children {
			btn := ui.NewButton(c.Name())
			btn.OnClicked(func(btn *ui.Button) {
				path := p.childMap[btn.Handle()]
				p.Lookup(path)
			})
			p.childMap[btn.Handle()] = c.Path()
			cols[i%len(cols)].Append(btn, false)
		}
		for _, col := range cols {
			colsGrp.Append(col, true)
		}
		p.contents.Append(colsGrp, false)
	}
	p.item = item
}

func (p *guiPage) ShowError(err error) {
	p.status.Show()
	p.status.SetText(err.Error())
}

func (p *guiPage) handleMessages(c <-chan lib.Message, setText bool) {
	for m := range c {
		switch m.(type) {
		case lib.MessageDone:
			item := m.(lib.MessageDone).Item().(lib.Item)
			p.SetItem(item, setText)
			p.status.Hide()
		default:
			p.status.Show()
			p.status.SetText(m.String())
		}
	}

}

func (p *guiPage) onPathChanged(sender *ui.Entry) {
	p.handleMessages(lib.LookupPath(p.lang, sender.Text()), false)
}