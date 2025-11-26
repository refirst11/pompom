package main

import (
	"fmt"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
)

type PomodoroTimer struct {
	window fyne.Window

	// Data binding (thread safe)
	timerText  binding.String
	statusText binding.String

	// Widget
	minutesEntry *widget.Entry
	startButton  *widget.Button
	pauseButton  *widget.Button
	resetButton  *widget.Button

	// Timer state
	workMinutes  int
	breakMinutes int
	remaining    int
	isRunning    bool
	isPaused     bool
	isWorkTime   bool

	// Control channel
	stopChan   chan bool
	pauseChan  chan bool
	resumeChan chan bool
}

func NewPomodoroTimer(w fyne.Window) *PomodoroTimer {
	timerText := binding.NewString()
	timerText.Set("25:00")

	statusText := binding.NewString()
	statusText.Set("Ready to start ! 🍅")

	p := &PomodoroTimer{
		window:       w,
		timerText:    timerText,
		statusText:   statusText,
		workMinutes:  25,
		breakMinutes: 5,
		stopChan:     make(chan bool),
		pauseChan:    make(chan bool),
		resumeChan:   make(chan bool),
	}

	p.createWidgets()
	return p
}

func (p *PomodoroTimer) createWidgets() {
	// Fraction input field
	p.minutesEntry = widget.NewEntry()
	p.minutesEntry.SetText("25")
	p.minutesEntry.SetPlaceHolder("Minutes")

	// Start button
	p.startButton = widget.NewButtonWithIcon("Start", theme.MediaPlayIcon(), func() {
		p.start()
	})
	p.startButton.Importance = widget.HighImportance

	// Stop button
	p.pauseButton = widget.NewButtonWithIcon("Pause", theme.MediaPauseIcon(), func() {
		p.pause()
	})
	p.pauseButton.Disable()

	// Reset button
	p.resetButton = widget.NewButtonWithIcon("Reset", theme.MediaStopIcon(), func() {
		p.reset()
	})
	p.resetButton.Disable()
}

func (p *PomodoroTimer) start() {
	if p.isRunning {
		return
	}

	//Get the input value
	text := p.minutesEntry.Text
	min, err := strconv.Atoi(text)
	if err != nil || min <= 0 {
		dialog.ShowError(fmt.Errorf("invalid minutes: please enter a positive number"), p.window)
		return
	}

	p.workMinutes = min
	p.remaining = min * 60
	p.isRunning = true
	p.isPaused = false
	p.isWorkTime = true

	// UI update
	p.startButton.Disable()
	p.pauseButton.Enable()
	p.resetButton.Enable()
	p.minutesEntry.Disable()
	p.statusText.Set("🍅 Work Time - Focus !")

	// Start the timer
	go p.runTimer()
}

func (p *PomodoroTimer) pause() {
	if !p.isRunning {
		return
	}

	if p.isPaused {
		// Resume
		p.isPaused = false
		p.resumeChan <- true
		p.pauseButton.SetText("Pause")
		p.pauseButton.SetIcon(theme.MediaPauseIcon())

		if p.isWorkTime {
			p.statusText.Set("🍅 Work Time - Focus !")
		} else {
			p.statusText.Set("☕ Break Time - Relax !")
		}
	} else {
		// Pause
		p.isPaused = true
		p.pauseChan <- true
		p.pauseButton.SetText("Resume")
		p.pauseButton.SetIcon(theme.MediaPlayIcon())
		p.statusText.Set("⏸️ Paused")
	}
}

func (p *PomodoroTimer) reset() {
	// Stop the timer
	if p.isRunning {
		select {
		case p.stopChan <- true:
		// Sent stop signal
		default:
			// Goroutine not listening, probably already stopped
		}
	}

	p.isRunning = false
	p.isPaused = false
	p.remaining = p.workMinutes * 60

	// UI update
	p.timerText.Set(fmt.Sprintf("%02d:00", p.workMinutes))
	p.statusText.Set("Ready to start ! 🍅")
	p.startButton.Enable()
	p.pauseButton.Disable()
	p.pauseButton.SetText("Pause")
	p.pauseButton.SetIcon(theme.MediaPauseIcon())
	p.resetButton.Disable()
	p.minutesEntry.Enable()
}

func (p *PomodoroTimer) runTimer() {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for p.remaining >= 0 {
		// Display the remaining time
		minutes := p.remaining / 60
		seconds := p.remaining % 60
		p.timerText.Set(fmt.Sprintf("%02d:%02d", minutes, seconds))

		if p.remaining == 0 {
			break
		}

		select {
		case <-p.stopChan:
			return

		case <-p.pauseChan:
			// Paused
			select {
			case <-p.resumeChan:
			// Resume
			case <-p.stopChan:
				return
			}

		case <-ticker.C:
			p.remaining--
		}
	}

	// Timer end
	p.handleTimerEnd()
}

func (p *PomodoroTimer) handleTimerEnd() {
	if p.isWorkTime {
		// End of work time
		p.timerText.Set("00:00")

		// Wait for 1 second
		time.Sleep(1 * time.Second)

		// Switch to break time
		p.startBreak()
	} else {
		// Break time is end
		p.timerText.Set("00:00")

		// Wait for 1 second
		time.Sleep(1 * time.Second)

		// Schedule the reset and start on the main thread to avoid race conditions
		fyne.Do(func() {
			p.reset()
			p.start()
		})
	}
}

func (p *PomodoroTimer) startBreak() {
	p.remaining = p.breakMinutes * 60
	p.isWorkTime = false
	p.isPaused = false
	p.statusText.Set("☕ Break Time - Relax !")

	go p.runTimer()
}

func (p *PomodoroTimer) buildUI() fyne.CanvasObject {
	title := widget.NewLabelWithStyle(
		"🍅 Pomodoro Timer",
		fyne.TextAlignCenter,
		fyne.TextStyle{Bold: true},
	)

	// Display timer (large)
	timerRich := widget.NewRichTextFromMarkdown("# 25:00")
	timerRich.Wrapping = fyne.TextWrapOff

	// Update with data binding
	p.timerText.AddListener(binding.NewDataListener(func() {
		text, _ := p.timerText.Get()
		timerRich.ParseMarkdown("# " + text)
	}))

	// Display status
	statusLabel := widget.NewLabelWithData(p.statusText)
	statusLabel.Alignment = fyne.TextAlignCenter
	statusLabel.Wrapping = fyne.TextWrapOff

	// Horizontally arrange the configuration sections
	settingsLabel := widget.NewLabel("Work Duration:")
	settingsRow := container.NewHBox(
		settingsLabel,
		p.minutesEntry,
		widget.NewLabel("minutes"), // Add unit display
	)
	settingsRow.Layout = layout.NewGridLayout(3) // 3 column grid layout

	// Button layout
	buttonRow := container.NewGridWithColumns(3,
		p.startButton,
		p.pauseButton,
		p.resetButton,
	)

	// Main layout
	content := container.NewVBox(
		container.NewCenter(title),
		layout.NewSpacer(),
		container.NewCenter(timerRich),
		layout.NewSpacer(),
		container.NewCenter(statusLabel),
		layout.NewSpacer(),
		widget.NewSeparator(),
		layout.NewSpacer(),
		container.NewCenter(settingsRow), // Center alignment
		layout.NewSpacer(),
		container.NewCenter(buttonRow), // Center alignment
		layout.NewSpacer(),
	)

	// Container with padding
	return container.NewPadded(content)
}

func main() {
	myApp := app.New()
	myWindow := myApp.NewWindow("pompom")

	pomodoro := NewPomodoroTimer(myWindow)

	myWindow.SetContent(pomodoro.buildUI())
	myWindow.Resize(fyne.NewSize(400, 500))
	myWindow.CenterOnScreen()

	myWindow.ShowAndRun()
}
