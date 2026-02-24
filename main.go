package main

import (
	"embed"
	"fmt"
	"image/color"
	"strconv"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/data/binding"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/layout"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"

	"github.com/faiface/beep"
	"github.com/faiface/beep/effects"
	"github.com/faiface/beep/mp3"
	"github.com/faiface/beep/speaker"
)

//go:embed notification.mp3
var soundFiles embed.FS

type PomodoroTimer struct {
	window     fyne.Window
	timerText  binding.String
	statusText binding.String

	minutesEntry *widget.Entry
	startButton  *orangeButton
	pauseButton  *widget.Button
	resetButton  *widget.Button

	workMinutes  int
	breakMinutes int
	remaining    int
	isRunning    bool
	isPaused     bool
	isWorkTime   bool
	breakDialog  dialog.Dialog

	stopChan   chan bool
	pauseChan  chan bool
	resumeChan chan bool

	volumeSlider *widget.Slider
	progress     binding.Float

	endTime     time.Time
	totalTasks  int
	pauseTime   time.Time
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
		progress:     binding.NewFloat(),
	}
	p.progress.Set(1.0)
	p.createWidgets()
	return p
}

func (p *PomodoroTimer) createWidgets() {
	p.minutesEntry = widget.NewEntry()
	p.minutesEntry.SetText("25")
	p.minutesEntry.SetPlaceHolder("Minutes")

	p.startButton = newOrangeButton("Start", theme.MediaPlayIcon(), func() {
		p.start()
	})

	p.pauseButton = widget.NewButtonWithIcon("Pause", theme.MediaPauseIcon(), func() {
		p.pause()
	})
	p.pauseButton.Disable()

	p.resetButton = widget.NewButtonWithIcon("Reset", theme.MediaStopIcon(), func() {
		p.reset()
	})
	p.resetButton.Disable()

	p.volumeSlider = widget.NewSlider(0, 100)
	p.volumeSlider.SetValue(50)
}

func (p *PomodoroTimer) start() {
	if p.isRunning {
		return
	}
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

	p.startButton.Disable()
	p.pauseButton.Enable()
	p.resetButton.Enable()
	p.minutesEntry.Disable()
	p.statusText.Set("🍅 Work Time - Focus !")
	p.progress.Set(1.0)

	p.endTime = time.Now().Add(time.Duration(p.workMinutes) * time.Minute)
	go p.runTimer()
}

func (p *PomodoroTimer) pause() {
	if !p.isRunning {
		return
	}
	if p.isPaused {
		p.isPaused = false
		p.endTime = p.endTime.Add(time.Since(p.pauseTime))

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
		p.isPaused = true
		p.pauseTime = time.Now()

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
	if p.isRunning {
		select {
		case p.stopChan <- true:
		default:
		}
	}
	if p.breakDialog != nil {
		p.breakDialog.Hide()
		p.breakDialog = nil
	}
	p.isRunning = false
	p.isPaused = false
	p.remaining = p.workMinutes * 60

	p.timerText.Set(fmt.Sprintf("%02d:00", p.workMinutes))
	p.statusText.Set("Ready to start ! 🍅")
	p.startButton.Enable()
	p.pauseButton.Disable()
	p.pauseButton.SetText("Pause")
	p.pauseButton.SetIcon(theme.MediaPauseIcon())
	p.resetButton.Disable()
	p.minutesEntry.Enable()
	p.progress.Set(1.0)
}

func (p *PomodoroTimer) runTimer() {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	lastSec := -1

	for {
		remainingDuration := time.Until(p.endTime)
		if remainingDuration <= 0 {
			p.remaining = 0
			p.progress.Set(0.0)
			break
		}

		p.remaining = int(remainingDuration.Seconds())
		minutes := p.remaining / 60
		seconds := p.remaining % 60

		if p.remaining != lastSec {
			p.timerText.Set(fmt.Sprintf("%02d:%02d", minutes, seconds))
			lastSec = p.remaining
		}

		totalSec := p.workMinutes * 60
		if !p.isWorkTime {
			totalSec = p.breakMinutes * 60
		}
		if totalSec > 0 {
			p.progress.Set(remainingDuration.Seconds() / float64(totalSec))
		}

		select {
		case <-p.stopChan:
			return
		case <-p.pauseChan:
			select {
			case <-p.resumeChan:
			case <-p.stopChan:
				return
			}
		case <-ticker.C:
		}
	}

	p.handleTimerEnd()
}

func (p *PomodoroTimer) handleTimerEnd() {
	if p.isWorkTime {
		p.timerText.Set("00:00")
		time.Sleep(1 * time.Second)
		p.startBreak()
	} else {
		p.timerText.Set("00:00")
		time.Sleep(1 * time.Second)
		fyne.Do(func() {
			if p.breakDialog != nil {
				p.breakDialog.Hide()
				p.breakDialog = nil
			}
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
			p.isRunning = false
			p.isPaused = false
			p.startButton.Enable()
			p.pauseButton.Disable()
			p.pauseButton.SetText("Pause")
			p.pauseButton.SetIcon(theme.MediaPauseIcon())
			p.resetButton.Disable()
			p.minutesEntry.Enable()
			p.timerText.Set(fmt.Sprintf("%02d:00", p.workMinutes))
			p.statusText.Set("Ready to start ! 🍅")
			fyne.CurrentApp().SendNotification(fyne.NewNotification("Work Time 🍅", "Break is over. Focus time!"))
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
	speaker.Init(format.SampleRate, format.SampleRate.N(time.Second/10))
	volume := &effects.Volume{
		Streamer: streamer,
		Base:     2,
		Volume:   (p.volumeSlider.Value / 20.0) - 5.0,
	}
	speaker.Play(beep.Seq(volume, beep.Callback(func() {
		streamer.Close()
	})))
}

func (p *PomodoroTimer) startBreak() {
	p.endTime = time.Now().Add(time.Duration(p.breakMinutes) * time.Minute)
	p.progress.Set(1.0)
	p.isWorkTime = false
	p.isPaused = false

	go p.playSound("notification.mp3")
	fyne.CurrentApp().SendNotification(fyne.NewNotification("Break Time ☕", "It's break time! Relax."))
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

	volLabel := widget.NewLabelWithStyle("", fyne.TextAlignTrailing, fyne.TextStyle{})
	p.volumeSlider.Step = 1.0
	volContainer := container.NewHBox(volLabel, container.NewGridWrap(fyne.NewSize(100, 40), p.volumeSlider))

	header := container.NewStack(
		container.NewCenter(title),
		container.NewHBox(layout.NewSpacer(), volContainer),
	)

	timerRich := widget.NewRichTextFromMarkdown("# 25:00")
	timerRich.Wrapping = fyne.TextWrapOff
	progressCircle := NewCircularProgress(p.progress)

	timerContainer := container.NewStack(
		container.NewCenter(progressCircle),
		container.NewCenter(timerRich),
	)

	p.timerText.AddListener(binding.NewDataListener(func() {
		text, _ := p.timerText.Get()
		timerRich.ParseMarkdown("# " + text)
	}))

	statusLabel := widget.NewLabelWithData(p.statusText)
	statusLabel.Alignment = fyne.TextAlignCenter
	statusLabel.Wrapping = fyne.TextWrapOff

	settingsLabel := widget.NewLabel("Work Duration:")
	settingsRow := container.NewHBox(
		settingsLabel,
		p.minutesEntry,
		widget.NewLabel("minutes"),
	)
	settingsRow.Layout = layout.NewGridLayout(3)

	buttonRow := container.NewGridWithColumns(3,
		p.startButton,
		p.pauseButton,
		p.resetButton,
	)

	content := container.NewVBox(
		header,
		layout.NewSpacer(),
		timerContainer,
		layout.NewSpacer(),
		container.NewCenter(statusLabel),
		layout.NewSpacer(),
		widget.NewSeparator(),
		layout.NewSpacer(),
		container.NewCenter(settingsRow),
		layout.NewSpacer(),
		container.NewCenter(buttonRow),
		layout.NewSpacer(),
	)

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

type CircularProgress struct {
	widget.BaseWidget
	Progress binding.Float
	arc      *canvas.Arc
}

func NewCircularProgress(progress binding.Float) *CircularProgress {
	cp := &CircularProgress{Progress: progress}
	cp.ExtendBaseWidget(cp)
	cp.arc = canvas.NewArc(0, 360, 0.96, color.RGBA{R: 255, G: 179, B: 71, A: 255})

	cp.Progress.AddListener(binding.NewDataListener(func() {
		val, _ := cp.Progress.Get()
		cp.arc.StartAngle = float32(360 * (1 - val))
		cp.arc.EndAngle = 360
		cp.Refresh()
	}))
	return cp
}

func (cp *CircularProgress) CreateRenderer() fyne.WidgetRenderer {
	return &circularProgressRenderer{cp: cp}
}

type circularProgressRenderer struct {
	cp *CircularProgress
}

func (r *circularProgressRenderer) Destroy() {}
func (r *circularProgressRenderer) Layout(size fyne.Size) {
	r.cp.arc.Resize(size)
}
func (r *circularProgressRenderer) MinSize() fyne.Size {
	return fyne.NewSize(220, 220)
}
func (r *circularProgressRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.cp.arc}
}
func (r *circularProgressRenderer) Refresh() {
	canvas.Refresh(r.cp)
}

type orangeButton struct {
	widget.BaseWidget
	label    string
	icon     fyne.Resource
	onTapped func()
	disabled bool
}

func newOrangeButton(label string, icon fyne.Resource, tapped func()) *orangeButton {
	b := &orangeButton{
		label:    label,
		icon:     icon,
		onTapped: tapped,
	}
	b.ExtendBaseWidget(b)
	return b
}

func (b *orangeButton) Tapped(_ *fyne.PointEvent) {
	if !b.disabled && b.onTapped != nil {
		b.onTapped()
	}
}

func (b *orangeButton) Enable() {
	b.disabled = false
	b.Refresh()
}

func (b *orangeButton) Disable() {
	b.disabled = true
	b.Refresh()
}

func (b *orangeButton) Disabled() bool {
	return b.disabled
}

func (b *orangeButton) CreateRenderer() fyne.WidgetRenderer {
	bg := canvas.NewRectangle(color.RGBA{R: 255, G: 179, B: 71, A: 255})
	bg.CornerRadius = 4

	iconImg := canvas.NewImageFromResource(b.icon)
	iconImg.SetMinSize(fyne.NewSize(20, 20))
	iconImg.FillMode = canvas.ImageFillContain

	label := canvas.NewText(b.label, theme.Color(theme.ColorNameForeground))
	label.TextStyle = fyne.TextStyle{Bold: true}
	label.Alignment = fyne.TextAlignCenter

	return &orangeButtonRenderer{
		bg:    bg,
		icon:  iconImg,
		label: label,
		btn:   b,
	}
}

type orangeButtonRenderer struct {
	bg    *canvas.Rectangle
	icon  *canvas.Image
	label *canvas.Text
	btn   *orangeButton
}

func (r *orangeButtonRenderer) Destroy() {}

func (r *orangeButtonRenderer) MinSize() fyne.Size {
	return fyne.NewSize(80, 36)
}

func (r *orangeButtonRenderer) Layout(size fyne.Size) {
	r.bg.Resize(size)

	iconSize := float32(20)
	gap := float32(4)
	padding := float32(8)

	labelWidth := fyne.MeasureText(r.label.Text, r.label.TextSize, r.label.TextStyle).Width
	totalW := iconSize + gap + labelWidth
	startX := (size.Width - totalW) / 2
	centerY := (size.Height - iconSize) / 2

	r.icon.Move(fyne.NewPos(startX, centerY))
	r.icon.Resize(fyne.NewSize(iconSize, iconSize))

	r.label.Move(fyne.NewPos(startX+iconSize+gap, padding))
	r.label.Resize(fyne.NewSize(labelWidth+4, size.Height-padding*2))
}

func (r *orangeButtonRenderer) Objects() []fyne.CanvasObject {
	return []fyne.CanvasObject{r.bg, r.icon, r.label}
}

func (r *orangeButtonRenderer) Refresh() {
	if r.btn.disabled {
		r.bg.FillColor = color.RGBA{R: 200, G: 200, B: 200, A: 255}
	} else {
		r.bg.FillColor = color.RGBA{R: 255, G: 179, B: 71, A: 255}
	}

	r.label.Color = theme.Color(theme.ColorNameForeground)
	r.bg.Refresh()
	r.icon.Refresh()
	r.label.Refresh()
}