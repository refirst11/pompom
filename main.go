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

	"embed"

	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
)

//go:embed notification.mp3
var soundFiles embed.FS

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
	breakDialog  dialog.Dialog

	// Control channel
	stopChan   chan bool
	pauseChan  chan bool
	resumeChan chan bool

	volumeSlider *widget.Slider
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
		stopChan:     make(chan bool, 1),
		pauseChan:    make(chan bool, 1),
		resumeChan:   make(chan bool, 1),
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

	// Volume slider
	p.volumeSlider = widget.NewSlider(0, 100)
	p.volumeSlider.SetValue(50)
	// Suggest a fixed width for the slider to look better in the corner
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
		select {
		case p.resumeChan <- true:
		default:
		}
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
		select {
		case p.pauseChan <- true:
		default:
		}
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

	if p.breakDialog != nil {
		p.breakDialog.Hide()
		p.breakDialog = nil
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

		// Schedule the restart on the main thread to avoid race conditions.
		fyne.Do(func() {
			if p.breakDialog != nil {
				p.breakDialog.Hide()
				p.breakDialog = nil
			}

			// Drain any stale signals from channels
			select {
			case <-p.stopChan:
			default:
			}
			select {
			case <-p.pauseChan:
			default:
			}
			select {
			case <-p.resumeChan:
			default:
			}

			// Reset state directly
			p.isRunning = false
			p.isPaused = false

			// UI reset
			p.startButton.Enable()
			p.pauseButton.Disable()
			p.pauseButton.SetText("Pause")
			p.pauseButton.SetIcon(theme.MediaPauseIcon())
			p.resetButton.Disable()
			p.minutesEntry.Enable()
			p.timerText.Set(fmt.Sprintf("%02d:00", p.workMinutes))
			p.statusText.Set("Ready to start ! 🍅")

			// Start next work session
			p.start()
		})
	}
}

func (p *PomodoroTimer) playSound(filename string) {
	f, err := soundFiles.Open(filename)
	if err != nil {
		fmt.Println("Error opening sound file:", err)
		return
	}

	streamer, format, err := mp3.Decode(f)
	if err != nil {
		fmt.Println("Error decoding mp3:", err)
		return
	}

	// Initialize speaker. Note: Init can be called multiple times, but 
	// we should ideally do it once or when the format changes.
	speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10))

	// Apply volume
	volume := &effects.Volume{
		Streamer: streamer,
		Base:     2,
		Volume:   (p.volumeSlider.Value / 20.0) - 5.0, // 100 -> 0, 0 -> -5
	}

	speaker.Play(beep.Seq(volume, beep.Callback(func() {
		streamer.Close()
	})))
}

func (p *PomodoroTimer) startBreak() {
	p.remaining = p.breakMinutes * 60
	p.isWorkTime = false
	p.isPaused = false

	// Play notification sound
	go p.playSound("notification.mp3")

    fyne.Do(func() {
        if p.volumeSlider.Value == 0 {
            if p.breakDialog != nil {
                p.breakDialog.Hide()
            }
            p.breakDialog = dialog.NewInformation(
                "Break Time ☕",
                "It's break time! Relax.",
                p.window,
            )
            p.breakDialog.Show()
        }
    })
	
	p.statusText.Set("☕ Break Time - Relax !")
	go p.runTimer()
}

func (p *PomodoroTimer) buildUI() fyne.CanvasObject {
	title := widget.NewLabelWithStyle(
		"🍅 Pomodoro Timer",
		fyne.TextAlignCenter,
		fyne.TextStyle{Bold: true},
	)

	// Volume control at the top right
	volLabel := widget.NewLabelWithStyle("", fyne.TextAlignTrailing, fyne.TextStyle{})
	p.volumeSlider.Step = 1.0
	// Set min size to the slider so it's not too small or too wide
	volContainer := container.NewHBox(volLabel, container.NewGridWrap(fyne.NewSize(100, 40), p.volumeSlider))
	
	header := container.NewStack(
		container.NewCenter(title),
		container.NewHBox(layout.NewSpacer(), volContainer),
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
		header,
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
