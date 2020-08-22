package main

import (
	"errors"
	"fmt"
	"log"
	"os/exec"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/gdamore/tcell"
	"github.com/rivo/tview"
)

// takes year/race ID and returns full year and race nuber as strings
func getYearAndRace(input string) (string, string, error) {
	var fullYear string
	var raceNumber string
	if len(input) < 4 {
		return fullYear, raceNumber, errors.New("not long enough")
	}
	_, err := strconv.Atoi(input[:4])
	if err != nil {
		return fullYear, raceNumber, errors.New("not a valid YearRaceID")
	}
	// TODO fix before 2020
	if input[:4] == "2018" || input[:4] == "2019" {
		return input[:4], "0", nil
	}
	year := input[:2]
	intYear, _ := strconv.Atoi(year)
	// TODO: change before 2030
	if intYear < 30 {
		fullYear = "20" + year
	} else {
		fullYear = "19" + year
	}
	raceNumber = input[2:4]
	return fullYear, raceNumber, nil
}

func (session *viewerSession) logError(v ...interface{}) {
	if session.textWindow != nil {
		fmt.Fprintln(session.textWindow, fmt.Sprintf("[%s::b]ERROR:[-::-]", colortoHexString(activeTheme.ErrorColor)), fmt.Sprint(v...))
	}
	log.Println("[ERROR]", fmt.Sprint(v...))
}

func (session *viewerSession) logInfo(v ...interface{}) {
	if session.textWindow != nil {
		fmt.Fprintln(session.textWindow, fmt.Sprintf("[%s::b]INFO:[-::-]", colortoHexString(activeTheme.InfoColor)), fmt.Sprint(v...))
	}
	log.Println("[INFO]", fmt.Sprint(v...))
}

func (session *viewerSession) withBlink(node *tview.TreeNode, fn func(), after func()) func() {
	return func() {
		done := make(chan struct{})
		go func() {
			fn()
			done <- struct{}{}
		}()
		go func() {
			session.blinkNode(node, done)
			if after != nil {
				after()
			}
		}()
	}
}

func (session *viewerSession) blinkNode(node *tview.TreeNode, done chan struct{}) {
	originalText := node.GetText()
	originalColor := node.GetColor()
	color1 := originalColor
	color2 := activeTheme.LoadingColor
	node.SetText("loading...")

	ticker := time.NewTicker(200 * time.Millisecond)
	for {
		select {
		case <-done:
			node.SetText(originalText)
			node.SetColor(originalColor)
			session.app.Draw()
			return
		case <-ticker.C:
			node.SetColor(color2)
			session.app.Draw()
			c := color1
			color1 = color2
			color2 = c
		}
	}
}

func hexStringToColor(hex string) tcell.Color {
	hex = strings.ReplaceAll(hex, "#", "")
	//TODO: check err?
	color, _ := strconv.ParseInt(hex, 16, 32)
	return tcell.NewHexColor(int32(color))
}

func colortoHexString(color tcell.Color) string {
	return fmt.Sprintf("#%06x", color.Hex())
}

func (t theme) apply() {
	if t.TerminalTextColor != "" {
		tview.Styles.PrimaryTextColor = hexStringToColor(t.TerminalTextColor)
	}
	if t.CategoryNodeColor != "" {
		activeTheme.CategoryNodeColor = hexStringToColor(t.CategoryNodeColor)
	}
	if t.FolderNodeColor != "" {
		activeTheme.FolderNodeColor = hexStringToColor(t.FolderNodeColor)
	}
	if t.ItemNodeColor != "" {
		activeTheme.ItemNodeColor = hexStringToColor(t.ItemNodeColor)
	}
	if t.ActionNodeColor != "" {
		activeTheme.ActionNodeColor = hexStringToColor(t.ActionNodeColor)
	}
	if t.BackgroundColor != "" {
		tview.Styles.PrimitiveBackgroundColor = hexStringToColor(t.BackgroundColor)
	}
	if t.BorderColor != "" {
		tview.Styles.BorderColor = hexStringToColor(t.BorderColor)
	}
	if t.NoContentColor != "" {
		activeTheme.NoContentColor = hexStringToColor(t.NoContentColor)
	}
	if t.LoadingColor != "" {
		activeTheme.LoadingColor = hexStringToColor(t.LoadingColor)
	}
	if t.LiveColor != "" {
		activeTheme.LiveColor = hexStringToColor(t.LiveColor)
	}
	if t.UpdateColor != "" {
		activeTheme.UpdateColor = hexStringToColor(t.UpdateColor)
	}
	if t.TerminalAccentColor != "" {
		activeTheme.TerminalAccentColor = hexStringToColor(t.TerminalAccentColor)
	}
	if t.TerminalTextColor != "" {
		activeTheme.TerminalTextColor = hexStringToColor(t.TerminalTextColor)
	}
	if t.InfoColor != "" {
		activeTheme.InfoColor = hexStringToColor(t.InfoColor)
	}
	if t.ErrorColor != "" {
		activeTheme.ErrorColor = hexStringToColor(t.ErrorColor)
	}
	if t.MultiCommandColor != "" {
		activeTheme.MultiCommandColor = hexStringToColor(t.MultiCommandColor)
	}
}

func sanitizeFileName(s string) string {
	whitespace := regexp.MustCompile(`\s+`)
	var illegal *regexp.Regexp
	if runtime.GOOS == "windows" {
		illegal = regexp.MustCompile(`[<>:"/\\|?*]`)
	} else {
		illegal = regexp.MustCompile(`/`)
	}
	s = illegal.ReplaceAllString(s, " ")
	s = whitespace.ReplaceAllString(s, " ")
	s = strings.TrimSpace(s)
	return s
}

// Hacky solution for a working copy paste on Windows
// Breaks encapsulation by accessing a private member using reflect
// remove once https://github.com/gdamore/tcell/issues/319 is fixed
func (session *viewerSession) enablePaste(field *tview.InputField, form *tview.Form) {
	if runtime.GOOS != "windows" {
		return
	}
	field.SetInputCapture(func(event *tcell.EventKey) *tcell.EventKey {
		if event.Key() == tcell.KeyCtrlV || event.Key() == tcell.KeyCtrlY {
			session.handlePaste(field)
		}
		return event
	})

	// paste on right click
	field.SetMouseCapture(func(action tview.MouseAction, event *tcell.EventMouse) (tview.MouseAction, *tcell.EventMouse) {
		item, _ := form.GetFocusedItemIndex()

		if item != -1 && event.Buttons()&tcell.Button2 != 0 && form.GetFormItem(item) == field {
			session.handlePaste(field)
		}

		return action, event
	})
}

func (session *viewerSession) handlePaste(field *tview.InputField) {
	clipContent, err := clipboard.ReadAll()
	if err != nil {
		session.logError("could not paste: ", err)
	}
	val := reflect.ValueOf(*field)
	cursorPos := val.FieldByName("cursorPos").Int()
	text := field.GetText()
	field.SetText(text[0:cursorPos] + clipContent + text[cursorPos:])
	// TODO: why does Draw() deadlock?
	session.app.ForceDraw()
}
