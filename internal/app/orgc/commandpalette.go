package orgc

import (
	"github.com/rivo/tview"
)

// 	"github.com/lithammer/fuzzysearch/fuzzy"

type CommandPalette struct {
	view    *tview.InputField
	core    *Core
	cmdText string
}

func NewCommandPalette(core *Core) *CommandPalette {
	cmd := new(CommandPalette)
	cmd.core = core
	cmd.view = tview.NewInputField()

	onSearch := func(text string) {
		cmd.cmdText = text
		//nodes := searchTree(tree, text)
		//copiedRootPointer.SetChildren(nodes)
	}

	//if len(tree.searchedText) > 0 {
	//	onSearch(tree.searchedText)
	//}

	cmd.
		view.
		SetLabel(":").
		SetText(cmd.cmdText).
		SetChangedFunc(onSearch)
		/*
			.
			SetDoneFunc(func(key tcell.Key) {
				if key == tcell.KeyEscape {
					if len(cmd.cmdText) <= 0 {
						//tree.setAllDisplayTextToBasename(tree.originalRootNode)
						//tree.SetRoot(tree.originalRootNode)
					}
					//window.removeSearch()
				}
			})*/
	return cmd
}

/*
func searchTree(tree *tree, text string) []*tview.TreeNode {
	nodes := []*tview.TreeNode{}
	for _, node := range lastNodes(tree.originalRootNode) {
		path := extractNodeReference(node).path
		if fuzzy.Match(text, path) {
			nodes = append(nodes, node)
		}
	}

	return nodes
}
*/
