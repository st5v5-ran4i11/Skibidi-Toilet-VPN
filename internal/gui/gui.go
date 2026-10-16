package gui

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/config"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/core/events"
	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/progress"

	"github.com/st5v5-ran4i11/Skibidi_toilet_VPN/internal/logger"
	webview "github.com/webview/webview_go"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
)

const indexHTML = `
<!DOCTYPE html>
<html>
<head>
	<meta charset="utf-8">

	<style>
		body {
			background-color: #202020;
			color: #FFFFFF;
			font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif;
			margin: 0;
			padding: 10px;
			min-width: 410px;
			box-sizing: border-box;
			user-select: none;
		}

		.title {
			font-size: 16px;
			font-weight: 600;
			text-align: center;
			margin-bottom: 10px;
		}

		.radio-group {
			display: flex;
			justify-content: space-around;
			background-color: #1A1A1A;
			border: 1px solid #2F2F2F;
			border-radius: 6px;
			padding: 8px;
			margin-bottom: 15px;
		}

		.radio-item {
			display: flex;
			align-items: center;
			font-size: 13px;
			cursor: pointer;
		}

		.radio-item input {
			accent-color: #60CDFF;
			margin-right: 6px;
			cursor: pointer;
		}

		.form-row {
			display: flex;
			align-items: center;
			justify-content: space-between;
			margin-bottom: 10px;
			height: 35px;
		}

		.form-row label {
			font-size: 13px;
			color: #FFFFFF;
		}

		.form-row input[type="number"] {
			background-color: #202020;
			color: #FFFFFF;
			border: 1.5px solid #2F2F2F;
			border-radius: 4px;
			padding: 4px 8px;
			width: 80px;
			text-align: center;
			font-size: 13.5px;
		}

		.share-row {
			background-color: #1A1A1A;
			border: 1px solid #2F2F2F;
			border-radius: 6px;
			padding: 10px;
			margin-top: 5px;
			margin-bottom: 15px;
			display: flex;
			align-items: center;
			cursor: pointer;
			font-size: 13px;
		}

		.share-row input {
			accent-color: #60CDFF;
			margin-right: 10px;
			transform: scale(1.1);
			cursor: pointer;
		}

		.section-label {
			font-size: 13px;
			color: #A0A0A0;
			margin: 12px 0 8px 0;
			font-weight: 500;
		}

		.grid-services {
			display: grid;
			grid-template-columns: 1fr 1fr;
			gap: 10px;
			margin-bottom: 15px;
			background-color: #1A1A1A;
			border: 1px solid #2F2F2F;
			border-radius: 6px;
			padding: 10px;
			min-height: 50px;
		}

		.checkbox-item {
			display: flex;
			align-items: center;
			font-size: 13px;
			cursor: pointer;
		}

		.checkbox-item input {
			accent-color: #60CDFF;
			margin-right: 8px;
			transform: scale(1.1);
			cursor: pointer;
		}

		.start-btn {
			background-color: #60CDFF;
			color: #000000;
			border: none;
			border-radius: 4px;
			width: 100%;
			height: 40px;
			font-size: 14px;
			font-weight: bold;
			cursor: pointer;
			margin-bottom: 15px;
			transition: background-color 0.2s;
		}

		.start-btn:disabled {
			background-color: #2F2F2F;
			color: #A0A0A0;
			cursor: not-allowed;
		}

		.stage-box {
			margin-bottom: 12px;
		}

		.stage-label {
			font-size: 12.5px;
			color: #A0A0A0;
			margin-bottom: 4px;
			display: flex;
			justify-content: space-between;
			align-items: flex-start;
			gap: 8px;
			flex-wrap: wrap;
		}

		.progress-track {
			background-color: #2F2F2F;
			border-radius: 3px;
			width: 100%;
			height: 5px;
			overflow: hidden;
		}

		.progress-bar {
			background-color: #60CDFF;
			width: 0%;
			height: 100%;
			transition: width 0.1s linear;
			position: relative;
			overflow: hidden;
		}

		/*
		 * Unknown total: не показываем ложные 55% заполнения.
		 * Полоса остаётся пустой, а по ней проходит только тонкий
		 * индикатор активности. Когда Total станет известен, JS
		 * автоматически переключит её на обычный процент.
		 */
		.progress-bar.unknown {
			width: 100%;
			background: transparent;
			animation: none;
		}

		.progress-bar.unknown::after {
			content: '';
			position: absolute;
			top: 0;
			left: -22%;
			width: 22%;
			height: 100%;
			background: rgba(96,205,255,0.95);
			filter: blur(0.5px);
			will-change: transform;
			animation: progress-sweep 1.2s ease-in-out infinite;
		}
		.progress-bar.unknown.paused::after { animation: none; display: none; }

		@keyframes progress-pulse {
			0%, 100% { opacity: 0.55; }
			50% { opacity: 1; }
		}

		@keyframes progress-sweep {
			0% { transform: translateX(0); }
			100% { transform: translateX(560%); }
		}

		.progress-bar.done {
			background-color: #28a745;
		}
	</style>
</head>

<body>

	<div class="title">Параметры Конвейера VPN</div>

	<div class="radio-group">
		<label class="radio-item">
			<input
				type="radio"
				name="netMode"
				id="modeHome"
				checked
				onchange="onRadioModeChange('home')"
			>
			<span>Домашний (Wi-Fi)</span>
		</label>

		<label class="radio-item">
			<input
				type="radio"
				name="netMode"
				id="modeMobile"
				onchange="onRadioModeChange('mobile')"
			>
			<span>Мобильный (4G/5G)</span>
		</label>
	</div>

	<div class="form-row">
		<label>Воркеры Fetcher (скачивание):</label>
		<input type="number" id="fetcherWorkers">
	</div>

	<div class="form-row">
		<label>Воркеры TCP (быстрый отсев):</label>
		<input type="number" id="tcpWorkers">
	</div>

	<div class="form-row">
		<label>Параллельно проверяемых нод:</label>
		<input type="number" id="deepParallel">
	</div>

	<div class="form-row">
		<label>Таймаут глубокого чека (сек):</label>
		<input type="number" id="deepTimeout">
	</div>

	<label class="share-row">
		<input type="checkbox" id="runShareServer">
		<span>Включить локальную раздачу подписок (share_server)</span>
	</label>

	<div class="section-label">Тестируемые сервисы</div>

	<div class="grid-services" id="servicesGrid"></div>

	<button
		class="start-btn"
		id="startBtn"
		onclick="goStartPipeline()"
	>
		СТАРТ
	</button>

	<div class="section-label">Состояние runtime</div>
	<div id="runtimeStats" style="font-size:12px;color:#A0A0A0;margin-bottom:12px;line-height:1.55">
		Источники: — · Nodes: — · TCP: — · Deep: — · Working: —<br>
		Active Services: — · Recovering: — · sing-box-lx: —
	</div>

	<div id="progressContainer"></div>
	<button class="start-btn" id="closeBtn" style="display:none;background:#b42323;color:#fff;margin-top:14px" onclick="window.backendClose()">ЗАВЕРШИТЬ И ЗАКРЫТЬ</button>

	<script>
		function setFieldValue(id, value) {
			var element = document.getElementById(id);

			if (element) {
				element.value = value;
			}
		}

		function setCheckboxState(id, checked) {
			var element = document.getElementById(id);

			if (element) {
				element.checked = checked;
			}
		}

		function setRadioState(mode) {
			document.getElementById("modeMobile").checked = mode === "mobile";
			document.getElementById("modeHome").checked = mode !== "mobile";
		}

		function onRadioModeChange(mode) {
			window.backendChangeProfile(mode);
		}

		function initServices(allServices, enabledServices) {
			var grid = document.getElementById("servicesGrid");

			grid.innerHTML = "";

			for (var i = 0; i < allServices.length; i++) {
				var service = allServices[i].toLowerCase();
				var checked = enabledServices.includes(service)
					? "checked"
					: "";

				grid.innerHTML +=
					"<label class='checkbox-item'>" +
						"<input type='checkbox' id='chk_" +
						service +
						"' " +
						checked +
						">" +
						"<span>" +
						allServices[i].toUpperCase() +
						"</span>" +
					"</label>";
			}
		}

		function updateRuntimeStats(data) {
			var e=document.getElementById("runtimeStats");
			if(!e)return;
		e.innerHTML =
			"Источники: "+(data.sources??"—")+" · Nodes: "+(data.nodes??"—")+" · TCP: "+(data.tcp??"—")+" · Deep: "+(data.deep??"—")+" · Working: "+(data.working??"—")+"<br>"+
			"Active Services: "+(data.activeServices??"—")+" · Recovering: "+(data.recoveringServices??"—")+" · sing-box-lx: "+(data.runningLX??"—");
		}

		function updateStage(name, percent, info) {
			var box = document.getElementById("box_" + name);

			if (!box) {
				var container =
					document.getElementById("progressContainer");

				container.innerHTML +=
					"<div class='stage-box' id='box_" + name + "'>" +
						"<div class='stage-label'>" +
							"<span>" +
								name +
							"</span>" +
							"<span id='text_" + name + "'>" +
								info +
							"</span>" +
						"</div>" +
						"<div class='progress-track'>" +
							"<div " +
								"class='progress-bar' " +
								"id='bar_" + name + "'>" +
							"</div>" +
						"</div>" +
					"</div>";

				box = document.getElementById("box_" + name);
			}

			var text = document.getElementById("text_" + name);
			var bar = document.getElementById("bar_" + name);

			if (text) {
				text.innerText = info;
			}

			if (bar) {
				if (percent < 0) {
					bar.classList.add("unknown");
					bar.classList.remove("done");
					bar.style.width = "100%";
				} else {
					bar.classList.remove("unknown");
					var safePercent =
						Math.max(0, Math.min(100, percent));
					bar.style.width = safePercent + "%";

					if (safePercent >= 100) {
						bar.classList.add("done");
					} else {
						bar.classList.remove("done");
					}
				}
			}
		}

		function setRunState(state) {
			var button = document.getElementById("startBtn");
			var closeBtn = document.getElementById("closeBtn");
			var active = state === "RUNNING" || state === "PAUSING" || state === "RESUMING";
			button.disabled = state === "PAUSING" || state === "RESUMING";
			button.innerText = state === "PAUSED" ? "ПРОДОЛЖИТЬ" : (active ? "ПАУЗА" : "СТАРТ");
			button.style.backgroundColor = state === "COMPLETED" ? "#28a745" : "#60CDFF";
			closeBtn.style.display = state === "COMPLETED" ? "block" : "none";
			setStagePaused(state === "PAUSED" || state === "COMPLETED" || state === "ERROR" || state === "IDLE");
			var disabled = active || state === "PAUSED" || state === "PAUSING" || state === "RESUMING";
			document.getElementById("modeHome").disabled = disabled;
			document.getElementById("modeMobile").disabled = disabled;
			document.getElementById("runShareServer").disabled = disabled;
		}
		function setStagePaused(paused) {
		document.querySelectorAll(".progress-bar.unknown").forEach(function(bar) {
			bar.classList.toggle("paused", paused);
		});
		}

		function goStartPipeline() {
			var state = window.currentRunState || "IDLE";
			if (state === "RUNNING") { window.backendPause(); return; }
			if (state === "PAUSED") { window.backendResume(); return; }
			if (state !== "IDLE" && state !== "COMPLETED" && state !== "ERROR") return;
			setRunState("RUNNING");

			var data = {
				fetcherWorkers:
					parseInt(
						document.getElementById("fetcherWorkers").value
					) || 0,

				tcp:
					parseInt(
						document.getElementById("tcpWorkers").value
					) || 0,

				parallel:
					parseInt(
						document.getElementById("deepParallel").value
					) || 0,

				timeout:
					parseInt(
						document.getElementById("deepTimeout").value
					) || 0,

				runShareServer:
					document.getElementById("runShareServer").checked,

				services: []
			};

			var checkboxes =
				document.querySelectorAll(
					"#servicesGrid input[type=checkbox]"
				);

			checkboxes.forEach(function(chk) {
				if (chk.checked) {
					data.services.push(
						chk.id.replace("chk_", "")
					);
				}
			});

			window.backendStart(
				JSON.stringify(data)
			);
		}
	</script>

</body>
</html>
`

type Controller interface {
	Start(context.Context) error
	Stop()
	Pause()
	Resume()
	Status() string
	Progress() *progress.Manager
	Events() *events.Bus
}

type GUI struct {
	cfg          *config.Config
	controller   Controller
	wv           webview.WebView
	mu           sync.Mutex
	isProcessing bool
}

type PipeData struct {
	FetcherWorkers int      `json:"fetcherWorkers"`
	Tcp            int      `json:"tcp"`
	Parallel       int      `json:"parallel"`
	Timeout        int      `json:"timeout"`
	RunShareServer bool     `json:"runShareServer"`
	Services       []string `json:"services"`
}

func New(cfg *config.Config, controller Controller) *GUI {
	return &GUI{cfg: cfg, controller: controller}
}

func (g *GUI) ShowAndRun() {
	g.wv = webview.New(false)
	defer g.wv.Destroy()

	g.wv.SetTitle("Конвейер VPN: Настройки и Мониторинг")
	g.wv.SetSize(400, 560, webview.HintNone)

	g.wv.Bind("backendStart", func(jsonData string) {
		g.handleBackendStart(jsonData)
	})

	g.wv.Bind("backendPause", func() {
		if g.controller != nil {
			g.controller.Pause()
		}
	})
	g.wv.Bind("backendResume", func() {
		if g.controller != nil {
			g.controller.Resume()
		}
	})
	g.wv.Bind("backendClose", func() {
		if g.controller != nil {
			g.controller.Stop()
		}
		if g.wv != nil {
			g.wv.Terminate()
		}
	})

	g.wv.Bind("backendChangeProfile", func(mode string) {
		g.mu.Lock()

		if !g.isProcessing {
			if mode == "mobile" {
				g.cfg.NetworkMode = "mobile"
			} else {
				g.cfg.NetworkMode = "home"
			}
		}

		g.mu.Unlock()

		g.wv.Dispatch(func() {
			g.updateUIFields()
		})
	})

	g.wv.SetHtml(indexHTML)
	g.wv.Eval(`window.currentRunState="IDLE";setRunState("IDLE")`)

	go func() {
		time.Sleep(200 * time.Millisecond)

		g.wv.Dispatch(func() {
			g.updateUIFields()
		})
	}()

	g.wv.Run()

	if g.controller != nil {
		g.controller.Stop()
	}
}

func (g *GUI) handleBackendStart(jsonData string) {
	var data PipeData

	if err := json.Unmarshal(
		[]byte(jsonData),
		&data,
	); err != nil {
		return
	}

	g.mu.Lock()

	if g.isProcessing {
		g.mu.Unlock()
		return
	}

	g.isProcessing = true
	g.mu.Unlock()

	if data.FetcherWorkers < 1 {
		data.FetcherWorkers = 1
	}

	if data.Tcp < 1 {
		data.Tcp = 1
	}

	if data.Parallel < 1 {
		data.Parallel = 1
	}

	if data.Timeout < 1 {
		data.Timeout = 1
	}

	g.mu.Lock()

	g.cfg.RunShareServer = data.RunShareServer

	services := append(
		[]string(nil),
		data.Services...,
	)

	if g.cfg.NetworkMode == "mobile" {
		g.cfg.MobileProfile.FetcherWorkers =
			data.FetcherWorkers

		g.cfg.MobileProfile.TCPWorkers =
			data.Tcp

		g.cfg.MobileProfile.DeepNodesParallel =
			data.Parallel

		g.cfg.MobileProfile.DeepTimeoutSec =
			data.Timeout

		g.cfg.MobileProfile.EnabledServices =
			services
	} else {
		g.cfg.HomeProfile.FetcherWorkers =
			data.FetcherWorkers

		g.cfg.HomeProfile.TCPWorkers =
			data.Tcp

		g.cfg.HomeProfile.DeepNodesParallel =
			data.Parallel

		g.cfg.HomeProfile.DeepTimeoutSec =
			data.Timeout

		g.cfg.HomeProfile.EnabledServices =
			services
	}

	g.mu.Unlock()

	g.saveConfig()

	if g.controller == nil {
		g.finishRun("ERROR")
		return
	}
	if err := g.controller.Start(context.Background()); err != nil {
		if logger.Log != nil {
			logger.Log.Error("не удалось запустить Engine", zap.Error(err))
		}
		g.finishRun("ERROR")
	} else {
		go g.runPipeline()
	}
}

func (g *GUI) saveConfig() {
	root, err := os.Getwd()
	if err != nil {
		return
	}

	path := filepath.Join(
		root,
		"configs",
		"settings.yaml",
	)

	data, err := yaml.Marshal(g.cfg)
	if err != nil {
		return
	}

	_ = os.WriteFile(
		path,
		data,
		0644,
	)
}

func (g *GUI) runPipeline() {
	var ev <-chan events.Event
	var unsubscribe func()
	if g.controller != nil && g.controller.Events() != nil {
		ev, unsubscribe = g.controller.Events().Subscribe(32)
		defer unsubscribe()
	}
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case event, ok := <-ev:
			if ok && (event.Type == events.Complete || event.Type == events.Error) {
				status := g.controller.Status()
				if status == "RUNNING" || status == "PAUSED" {
					status = "COMPLETED"
				}
				g.finishRun(status)
				return
			}
		case <-ticker.C:
			if g.controller == nil {
				return
			}
			status := g.controller.Status()
			g.wv.Dispatch(func() {
				g.wv.Eval(fmt.Sprintf("window.currentRunState=%q;setRunState(%q);setStagePaused(%t)", status, status, status == "PAUSED"))
			})
			if status == "COMPLETED" || status == "ERROR" || status == "IDLE" {
				g.finishRun(status)
				return
			}
			g.updateProgress()
		}
	}
}

func (g *GUI) finishRun(status string) {
	g.mu.Lock()

	if !g.isProcessing {
		g.mu.Unlock()
		return
	}

	g.isProcessing = false

	g.mu.Unlock()

	if g.wv != nil {
		g.wv.Dispatch(func() {
			g.wv.Eval(fmt.Sprintf("window.currentRunState=%q;setRunState(%q);setStagePaused(true)", status, status))
		})
	}
}

// combineProcessingStages сводит три внутренних потоковых компонента в одну строку GUI.
// В агрегате показываем PARSER counters (число реально обработанных и успешно
// распознанных узлов), а состояние отражает работу всех трёх компонентов.
func combineProcessingStages(snapshot []progress.Stage) []progress.Stage {
	var collector, processorStage, parser progress.Stage
	found, inserted := false, false
	filtered := make([]progress.Stage, 0, len(snapshot)-2)
	for _, st := range snapshot {
		switch st.Name {
		case "COLLECTOR":
			collector = st
			found = true
		case "PROCESSOR":
			processorStage = st
			found = true
		case "PARSER":
			parser = st
			found = true
		}
	}
	if !found {
		return append([]progress.Stage(nil), snapshot...)
	}
	agg := parser
	agg.Name = "COLLECTOR / PROCESSOR / PARSER"
	agg.Total = progress.UnknownTotal
	agg.Workers = collector.Workers + processorStage.Workers + parser.Workers
	agg.Active = collector.Active || processorStage.Active || parser.Active
	agg.Done = collector.Done && processorStage.Done && parser.Done
	// Счётчики берем у PARSER: это число обработанных узлов, а не сумма
	// трёх стадий, которая повторно посчитала бы одни и те же узлы.
	agg.Processed, agg.Success, agg.Failed = parser.Processed, parser.Success, parser.Failed
	agg.Produced, agg.Alive = parser.Produced, parser.Alive
	agg.State = progress.Waiting
	for _, st := range []progress.Stage{collector, processorStage, parser} {
		if st.State == progress.Paused {
			agg.State = progress.Paused
			break
		}
		if st.State == progress.FailedState {
			agg.State = progress.FailedState
			continue
		}
		if st.State == progress.Running || st.State == progress.Stalled {
			agg.State = st.State
		}
	}
	if agg.Done {
		agg.State = progress.Completed
	}
	for _, st := range snapshot {
		switch st.Name {
		case "COLLECTOR", "PROCESSOR", "PARSER":
			if !inserted {
				filtered = append(filtered, agg)
				inserted = true
			}
		default:
			filtered = append(filtered, st)
		}
	}
	return filtered
}

func (g *GUI) updateProgress() {
	g.mu.Lock()

	if !g.isProcessing ||
		g.controller == nil ||
		g.wv == nil {
		g.mu.Unlock()
		return
	}

	wv := g.wv
	controller := g.controller

	g.mu.Unlock()

	snapshot := combineProcessingStages(controller.Progress().Snapshot())

	stats := map[string]int{}
	for _, st := range snapshot {
		switch st.Name {
		case "FETCHER":
			stats["sources"] = st.Processed
		case "TCP CHECK":
			stats["tcp"] = st.Success
		case "DEEP CHECK":
			stats["deep"] = st.Success
		}
	}
	if st, ok := findStage(snapshot, "COLLECTOR / PROCESSOR / PARSER"); ok {
		stats["nodes"] = st.Success
	}
	if rs, ok := controller.(interface{ RuntimeStats() map[string]int }); ok {
		for k, v := range rs.RuntimeStats() {
			stats[k] = v
		}
	}
	wv.Dispatch(func() { wv.Eval(fmt.Sprintf("updateRuntimeStats(%s)", mustJSON(stats))) })

	for _, stage := range snapshot {
		switch stage.Name {
		case "FETCHER", "COLLECTOR / PROCESSOR / PARSER", "TCP CHECK", "DEEP CHECK":
		default:
			continue
		}

		percent := 0
		info := "Ожидание..."

		if stage.State == progress.Waiting {
			percent = 0
			info = "Ожидание запуска этапа"
		} else if stage.State == progress.Paused {
			percent = -1
			info = fmt.Sprintf("Пауза · Обработано: %d · Успешно: %d", stage.Processed, stage.Success)
		} else if stage.Total == progress.UnknownTotal {
			percent = -1
			info = fmt.Sprintf("Обработано: %d · Успешно: %d", stage.Processed, stage.Success)
		} else if stage.Total > 0 {
			percent = stage.Processed * 100 / stage.Total
			if percent > 100 {
				percent = 100
			}
			info = fmt.Sprintf("%d%% · Обработано: %d/%d · Успешно: %d", percent, stage.Processed, stage.Total, stage.Success)
		} else if stage.Name == "FETCHER" {
			info = fmt.Sprintf("Источников: %d · Успешно: %d", stage.Processed, stage.Success)
		} else {
			info = fmt.Sprintf("Обработано: %d · Успешно: %d", stage.Processed, stage.Success)
		}
		if stage.Done {
			percent = 100
			info = fmt.Sprintf("Завершено · Обработано: %d · Успешно: %d", stage.Processed, stage.Success)
		}

		stageName := stage.Name
		stagePercent := percent
		stageInfo := info

		wv.Dispatch(func() {
			js := fmt.Sprintf(
				"updateStage(%q, %d, %q)",
				stageName,
				stagePercent,
				stageInfo,
			)

			wv.Eval(js)
		})
	}
}

func (g *GUI) updateUIFields() {
	g.mu.Lock()

	if g.wv == nil ||
		g.cfg == nil {
		g.mu.Unlock()
		return
	}

	wv := g.wv
	cfg := g.cfg

	profile := cfg.GetActiveProfile()

	networkMode := cfg.NetworkMode
	runShareServer := cfg.RunShareServer

	fetcherWorkers := profile.FetcherWorkers
	tcpWorkers := profile.TCPWorkers
	deepParallel := profile.DeepNodesParallel
	deepTimeout := profile.DeepTimeoutSec

	enabledServices := append(
		[]string(nil),
		profile.EnabledServices...,
	)

	allServices := make(
		[]string,
		0,
		len(cfg.AllServices),
	)

	for service := range cfg.AllServices {
		allServices = append(
			allServices,
			service,
		)
	}

	g.mu.Unlock()

	sort.Strings(allServices)

	allServicesJSON, err :=
		json.Marshal(allServices)

	if err != nil {
		return
	}

	enabledServicesJSON, err :=
		json.Marshal(enabledServices)

	if err != nil {
		return
	}

	wv.Eval(fmt.Sprintf(
		"setRadioState(%q)",
		networkMode,
	))

	wv.Eval(fmt.Sprintf(
		"setFieldValue('fetcherWorkers', '%d')",
		fetcherWorkers,
	))

	wv.Eval(fmt.Sprintf(
		"setFieldValue('tcpWorkers', '%d')",
		tcpWorkers,
	))

	wv.Eval(fmt.Sprintf(
		"setFieldValue('deepParallel', '%d')",
		deepParallel,
	))

	wv.Eval(fmt.Sprintf(
		"setFieldValue('deepTimeout', '%d')",
		deepTimeout,
	))

	wv.Eval(fmt.Sprintf(
		"setCheckboxState('runShareServer', %t)",
		runShareServer,
	))

	wv.Eval(fmt.Sprintf(
		"initServices(%s, %s)",
		string(allServicesJSON),
		string(enabledServicesJSON),
	))
}

func findStage(stages []progress.Stage, name string) (progress.Stage, bool) {
	for _, s := range stages {
		if s.Name == name {
			return s, true
		}
	}
	return progress.Stage{}, false
}
func mustJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
