import Cocoa

// GoviView is the editor surface: a custom NSView that draws one embedded
// engine's composed character grid and forwards key events into it. The engine
// runs in-process (via libgovi); this view is the "terminal" it renders to.
// Each window has its own GoviView bound to its own engine via `handle`.
final class GoviView: NSView, NSTextInputClient {
    // Engine-event modifier bits (must match engine.Mod).
    private static let modCtrl: Int32 = 1
    private static let modAlt: Int32 = 2

    // Special-key codes (must match engine.SpecialKey).
    private enum SK {
        static let escape: Int32 = 1
        static let enter: Int32 = 2
        static let backspace: Int32 = 4
        static let delete: Int32 = 5
        static let up: Int32 = 6
        static let down: Int32 = 7
        static let left: Int32 = 8
        static let right: Int32 = 9
        static let home: Int32 = 10
        static let end: Int32 = 11
        static let pageUp: Int32 = 12
        static let pageDown: Int32 = 13
    }

    // The handle of this view's embedded engine (one per window).
    let handle: Int64

    // documentTitle is the file name shown in the window title bar.
    var documentTitle = "Untitled"

    // Pending marked (uncommitted) text from a dead key or IME, e.g. the "¨"
    // after Option-u, shown at the cursor until the next key composes it.
    private var markedText = ""

    private var font = Settings.editorFont
    private var characterSpacing = Settings.characterSpacing
    private var lineSpacing = Settings.lineSpacing
    private var cellW: CGFloat = 8
    private var cellH: CGFloat = 16

    // Per-tab color specs, synced from the engine (:set / .exrc). Empty = system default.
    private var foregroundColorSpec = ""
    private var backgroundColorSpec = ""

    private var bgColor = NSColor.textBackgroundColor
    private var fgColor = NSColor.textColor
    private var cursorColor: NSColor { Settings.cursorColor }

    // Inset (pixels) between the window edge and the text grid, from Settings.
    private var padding: CGFloat = Settings.padding

    private var rows = 1
    private var cols = 1
    private var timer: Timer?

    // engineQueue serializes this window's engine input off the main thread (see
    // engineInput). A keystroke can launch a long command (search, :s, :g, or a
    // blocking :! ); running it on the main thread would freeze the UI and block
    // the very ^C meant to abort it. Each window has its own queue so windows run
    // independently; the engine itself stays single-threaded per window.
    private let engineQueue = DispatchQueue(label: "org.govi.engine")

    // One selection model, in screen cells. screenDragAnchor is the fixed origin
    // of a character drag; granLo/granHi are the originating word/line edges for
    // word/line drags; screenSelStart/End hold the stored highlighted span.
    // selBlock is an Option-drag rectangle (copy-only). selWholeBuffer is the
    // editor Select All, held as buffer carets (bufA/bufB) so it can cover text
    // scrolled off screen. Highlight and copy read the grid; an edit derives a
    // buffer range via GoviSelectionEditRange and beeps when the selection is not
    // wholly buffer text.
    private typealias Caret = (line: Int64, col: Int)
    private typealias ScreenCell = (x: Int32, y: Int32)
    private var selActive = false
    private var selBlock = false
    private var selWholeBuffer = false
    private var dragBlock = false
    private var screenSelStart: ScreenCell = (0, 0)
    private var screenSelEnd: ScreenCell = (0, 0)
    private var screenDragAnchor: ScreenCell = (0, 0)
    private var granLo: ScreenCell = (0, 0)
    private var granHi: ScreenCell = (0, 0)
    private var bufA: Caret = (1, 0)
    private var bufB: Caret = (1, 0)
    private var dragging = false

    // A buffer-anchored screen selection: a linear (non-block) selection lying
    // entirely on buffer text, kept in buffer coordinates so its highlight
    // follows the text when the view scrolls. reanchorSelection recomputes the
    // screen cells from these before every compose. selEdit is the half-open
    // [a, b) buffer range used by cut/copy/edit; selDrawA/selDrawB are the
    // inclusive endpoint carets used to redraw the highlight. Block rectangles
    // and selections touching non-buffer rows stay fixed to their screen cells.
    private var selBufBacked = false
    private var selEdit: (a: Caret, b: Caret) = ((1, 0), (1, 0))
    private var selDrawA: Caret = (1, 0)
    private var selDrawB: Caret = (1, 0)

    // selGranularity records how the selection was started; shift-extend and
    // drag-extend snap to word or line boundaries after double/triple-click.
    private enum SelGranularity { case character, word, line }
    private var selGranularity: SelGranularity = .character

    // Panes mirror the engine's split-screen layout (GoviPaneGeom /
    // GoviPaneScrollInfo), refreshed after every recompose. Each pane gets its
    // own overlay scroller; dividers between panes are draggable.
    private struct Pane {
        var roff = 0, coff = 0, rows = 0, cols = 0
        var active = false
        var top: Int64 = 1
        var lines: Int64 = 0
        var viewRows = 0
        var scrollable = false
    }
    private var panes: [Pane] = []
    private var scrollers: [NSScroller] = []

    // Overlay-style scrollers flash in when the content scrolls and fade back
    // out (like NSScrollView's); with the legacy preferred style they stay
    // visible. scrollersVisible is the flashed-in state.
    private var scrollersVisible = false
    private var scrollerFadeTimer: Timer?

    // An active divider drag: which pane's divider (below it for .rows, right
    // of it for .cols), the cell the drag started on, and the pane's size at
    // that moment. Each mouseDragged resizes toward start size + travel, so the
    // divider tracks the pointer and sticks at the engine's minimums.
    private enum DividerKind { case rows, cols }
    private var dividerDrag: (kind: DividerKind, pane: Int, startCell: Int, startSize: Int)?

    // Spell checking (continuous): red squiggles under misspelled words on the
    // visible lines. Results are cached by line text so unchanged lines are not
    // re-checked. Ranges are rune (Unicode scalar) indices, matching engine cols.
    private typealias RuneRange = (start: Int, end: Int)
    private struct Misspelling { let line: Int64; let start: Int; let end: Int }
    private var spellEnabled = Settings.spellChecking
    private let spellTag = NSSpellChecker.uniqueSpellDocumentTag()
    private var spellCache: [String: [RuneRange]] = [:]
    private var misspellings: [Misspelling] = []
    private var contextMisspelling: Misspelling?

    init(frame frameRect: NSRect, handle: Int64) {
        self.handle = handle
        super.init(frame: frameRect)
        measureFont()
        observeSettings()
    }
    required init?(coder: NSCoder) {
        self.handle = 0
        super.init(coder: coder)
        measureFont()
        observeSettings()
    }

    deinit {
        NotificationCenter.default.removeObserver(self)
    }

    private func observeSettings() {
        NotificationCenter.default.addObserver(
            self, selector: #selector(settingsChanged), name: Settings.changed, object: nil)
        // A closing window may be a sibling tab: reconcile the tab bar and the
        // mode indicator once the close has landed, not on the next keystroke.
        NotificationCenter.default.addObserver(
            self, selector: #selector(someWindowClosed), name: NSWindow.willCloseNotification,
            object: nil)
    }

    @objc private func someWindowClosed(_ note: Notification) {
        DispatchQueue.main.async { [weak self] in self?.updateTitle() }
    }

    private func applyColors() {
        fgColor = ColorParser.parse(foregroundColorSpec) ?? NSColor.textColor
        bgColor = ColorParser.parse(backgroundColorSpec) ?? NSColor.textBackgroundColor
    }

    private func syncColorsFromEngine() {
        var changed = false
        if let c = GoviForegroundSpec(handle) {
            let spec = String(cString: c)
            GoviFree(c)
            if spec != foregroundColorSpec {
                foregroundColorSpec = spec
                changed = true
            }
        }
        if let c = GoviBackgroundSpec(handle) {
            let spec = String(cString: c)
            GoviFree(c)
            if spec != backgroundColorSpec {
                backgroundColorSpec = spec
                changed = true
            }
        }
        if changed {
            applyColors()
        }
    }

    @objc private func settingsChanged() {
        padding = Settings.padding
        spellEnabled = Settings.spellChecking
        GoviSetMode(handle, Settings.selectionMode.code) // live-update this tab's mode
        font = Settings.editorFont
        characterSpacing = Settings.characterSpacing
        lineSpacing = Settings.lineSpacing
        measureFont()
        resizeWindowToCells() // keep the same rows x cols; grow/shrink the window to fit
        updateGeometry()      // refits only if the window could not take the target size
        updateSpelling()
        syncPanes()           // scroller frames follow the new cell metrics
        updateTitle()
        needsDisplay = true
    }

    // textRows is the editable line count (the status line is excluded).
    var textRows: Int { max(1, rows - 1) }

    // contentSize returns the view size needed for textRows x cols of editor text
    // plus one status line, using the given font metrics and padding.
    static func contentSize(
        textRows: Int, cols: Int,
        font: NSFont = Settings.editorFont, padding: CGFloat = Settings.padding,
        characterSpacing: CGFloat = Settings.characterSpacing,
        lineSpacing: CGFloat = Settings.lineSpacing
    ) -> NSSize {
        let metrics = Settings.cellSize(
            font: font, characterSpacing: characterSpacing, lineSpacing: lineSpacing)
        let cellW = metrics.cellW
        let cellH = metrics.cellH
        let screenRows = textRows + 1
        // Round up: AppKit rounds the content rect to whole points, and rounding
        // down would shave a partial cell and lose a row/column.
        return NSSize(
            width: ceil(padding * 2 + CGFloat(cols) * cellW),
            height: ceil(padding * 2 + CGFloat(screenRows) * cellH)
        )
    }

    // cellPoint / cellRect convert a (col, row) cell to view pixels, applying the
    // padding inset; cellOf is the inverse for hit-testing.
    private func cellPoint(_ col: Int, _ row: Int) -> NSPoint {
        NSPoint(x: padding + CGFloat(col) * cellW, y: padding + CGFloat(row) * cellH)
    }

    private func cellRect(_ col: Int, _ row: Int) -> NSRect {
        NSRect(origin: cellPoint(col, row), size: NSSize(width: cellW, height: cellH))
    }

    private func measureFont() {
        let metrics = Settings.cellSize(
            font: font, characterSpacing: characterSpacing, lineSpacing: lineSpacing)
        cellW = metrics.cellW
        cellH = metrics.cellH
    }

    // resizeWindowToCells sizes the window to keep the current rows x cols of
    // text after a font or padding change, instead of re-fitting the grid into
    // the old pixel size. The top-left corner stays fixed. No-op without a window
    // (updateGeometry then handles the fallback).
    private func resizeWindowToCells() {
        guard let window = window else { return }
        let content = GoviView.contentSize(
            textRows: textRows, cols: cols, font: font, padding: padding,
            characterSpacing: characterSpacing, lineSpacing: lineSpacing)
        let newFrameSize = window.frameRect(forContentRect: NSRect(origin: .zero, size: content)).size
        var frame = window.frame
        frame.origin.y += frame.size.height - newFrameSize.height // keep the top edge put
        frame.size = newFrameSize
        window.setFrame(frame, display: true)
    }

    // The grid's first row is at the top, so use a flipped coordinate system.
    override var isFlipped: Bool { true }
    override var acceptsFirstResponder: Bool { true }

    override func viewDidMoveToWindow() {
        super.viewDidMoveToWindow()
        window?.makeFirstResponder(self)
        updateGeometry()
    }

    // shiftDown reports whether Shift is held for a mouse event. AppKit
    // occasionally drops .shift from a mouseDown's own modifierFlags (notably the
    // synthesized second click of a double-click-then-shift-click), so fall back
    // to the current global modifier state and then the HID hardware state. This
    // is a live query each time -- no latched flags or flagsChanged tracking.
    private func shiftDown(_ event: NSEvent) -> Bool {
        let mask = NSEvent.ModifierFlags.deviceIndependentFlagsMask
        if event.modifierFlags.intersection(mask).contains(.shift) { return true }
        if NSEvent.modifierFlags.intersection(mask).contains(.shift) { return true }
        return CGEventSource.flagsState(.hidSystemState).contains(.maskShift)
    }

    override func setFrameSize(_ newSize: NSSize) {
        super.setFrameSize(newSize)
        updateGeometry()
    }

    // updateGeometry recomputes the cell rows/cols for the current bounds and
    // tells the engine, then recomposes and repaints.
    func updateGeometry() {
        // Add a small epsilon so an exact fit isn't lost to sub-pixel rounding
        // (e.g. 80*cellW coming back as 79.9999.../cellW).
        let w = max(1, Int((bounds.width - 2 * padding) / cellW + 1e-6))
        let h = max(1, Int((bounds.height - 2 * padding) / cellH + 1e-6))
        if w == cols && h == rows { return }
        cols = w
        rows = h
        GoviResize(handle, Int32(rows), Int32(cols))
        recompose()
        updateTitle()
        needsDisplay = true
    }

    // MARK: - Engine step

    // step is run after every input: it handles quit, bell, title, recomposes
    // the grid, repaints, and arms any pending timer (map/showmatch/recovery).
    func step() {
        if window?.isKeyWindow == true {
            syncWorkingDirectory()
        }
        if GoviShouldQuit(handle) != 0 {
            // windowShouldClose prompts when there are unsaved changes.
            window?.close()
            return
        }
        if GoviTakeBell(handle) != 0 {
            NSSound.beep()
        }
        updateTitle()
        recompose()
        needsDisplay = true
        armTimer()
    }

    private func recompose() {
        reanchorSelection() // follow the text if the viewport scrolled
        GoviCompose(handle, Int32(rows), Int32(cols))
        syncColorsFromEngine()
        updateSpelling()
        syncPanes()
    }

    // MARK: - Panes and scroll bars

    // syncPanes refreshes the pane layout and scroll state from the engine,
    // updates the per-pane scrollers, and flashes them when content scrolled.
    private func syncPanes() {
        var new: [Pane] = []
        let n = Int(GoviPaneCount(handle))
        for i in 0..<n {
            var roff: Int32 = 0, coff: Int32 = 0, prows: Int32 = 0, pcols: Int32 = 0
            var active: Int32 = 0
            guard GoviPaneGeom(handle, Int32(i), &roff, &coff, &prows, &pcols, &active) != 0 else { continue }
            var top: Int64 = 0, lines: Int64 = 0
            var viewRows: Int32 = 0, scrollable: Int32 = 0
            guard GoviPaneScrollInfo(handle, Int32(i), &top, &lines, &viewRows, &scrollable) != 0 else { continue }
            new.append(Pane(roff: Int(roff), coff: Int(coff), rows: Int(prows), cols: Int(pcols),
                            active: active != 0, top: top, lines: lines,
                            viewRows: Int(viewRows), scrollable: scrollable != 0))
        }
        let scrolled = new.count != panes.count || new.map(\.top) != panes.map(\.top)
        panes = new
        if scrolled {
            flashScrollers() // calls syncScrollers
        } else {
            syncScrollers()
        }
        window?.invalidateCursorRects(for: self)
    }

    // syncScrollers keeps one vertical NSScroller per pane, framed over the
    // right edge of the pane's text area, with the knob reflecting the pane's
    // viewport. Scrollers are overlay-style over the character grid, so the
    // engine's cell layout never changes.
    private func syncScrollers() {
        while scrollers.count < panes.count {
            let s = NSScroller(frame: NSRect(x: 0, y: 0, width: 10, height: 100))
            s.isEnabled = true
            s.target = self
            s.action = #selector(scrollerChanged(_:))
            s.controlSize = .regular
            s.scrollerStyle = .overlay
            s.tag = scrollers.count
            addSubview(s)
            scrollers.append(s)
        }
        while scrollers.count > panes.count {
            scrollers.removeLast().removeFromSuperview()
        }
        let overlay = NSScroller.preferredScrollerStyle == .overlay
        let hideAll = GoviExActive(handle) != 0 || GoviOverlayActive(handle) != 0
        let w = NSScroller.scrollerWidth(for: .regular, scrollerStyle: .overlay)
        for (i, pane) in panes.enumerated() {
            let s = scrollers[i]
            s.frame = NSRect(x: padding + CGFloat(pane.coff + pane.cols) * cellW - w,
                             y: padding + CGFloat(pane.roff) * cellH,
                             width: w, height: CGFloat(pane.rows) * cellH)
            let needed = pane.scrollable && !hideAll && pane.lines > Int64(pane.viewRows)
            s.isHidden = !needed || (overlay && !scrollersVisible)
            if !s.isHidden { s.alphaValue = 1 }
            s.knobStyle = darkBackground() ? .light : .dark
            if pane.lines > 0 {
                s.knobProportion = CGFloat(min(1.0, Double(pane.viewRows) / Double(pane.lines)))
                let denom = max(Int64(1), pane.lines - Int64(pane.viewRows))
                s.doubleValue = min(1.0, max(0.0, Double(pane.top - 1) / Double(denom)))
            }
        }
    }

    private func darkBackground() -> Bool {
        let rgb = bgColor.usingColorSpace(.deviceRGB) ?? bgColor
        var brightness: CGFloat = 1
        rgb.getHue(nil, saturation: nil, brightness: &brightness, alpha: nil)
        return brightness < 0.5
    }

    // flashScrollers shows the overlay scrollers and re-arms their fade-out.
    // With the legacy preferred style they are simply kept visible.
    private func flashScrollers() {
        scrollersVisible = true
        syncScrollers()
        scrollerFadeTimer?.invalidate()
        scrollerFadeTimer = nil
        guard NSScroller.preferredScrollerStyle == .overlay else { return }
        scrollerFadeTimer = Timer.scheduledTimer(withTimeInterval: 1.2, repeats: false) { [weak self] _ in
            self?.fadeOutScrollers()
        }
    }

    private func fadeOutScrollers() {
        scrollersVisible = false
        NSAnimationContext.runAnimationGroup({ ctx in
            ctx.duration = 0.25
            for s in scrollers { s.animator().alphaValue = 0 }
        }, completionHandler: { [weak self] in
            guard let self = self, !self.scrollersVisible else { return }
            for s in self.scrollers { s.isHidden = true }
        })
    }

    // scrollerChanged is the scrollers' action: the knob (and clicks in the
    // slot) position the pane's viewport absolutely; track/arrow parts page or
    // step it. The cursor stays put throughout, like any macOS scroll.
    @objc private func scrollerChanged(_ sender: NSScroller) {
        let i = sender.tag
        guard i >= 0 && i < panes.count else { return }
        let pane = panes[i]
        let page = Int32(max(1, pane.viewRows - 1))
        switch sender.hitPart {
        case .knob, .knobSlot:
            let denom = max(Int64(1), pane.lines - Int64(pane.viewRows))
            let top = Int64((sender.doubleValue * Double(denom)).rounded()) + 1
            engineInput { GoviPaneSetTop(self.handle, Int32(i), top) }
        case .decrementPage:
            engineInput { GoviPaneScrollBy(self.handle, Int32(i), -page) }
        case .incrementPage:
            engineInput { GoviPaneScrollBy(self.handle, Int32(i), page) }
        case .decrementLine:
            engineInput { GoviPaneScrollBy(self.handle, Int32(i), -1) }
        case .incrementLine:
            engineInput { GoviPaneScrollBy(self.handle, Int32(i), 1) }
        default:
            break
        }
        flashScrollers()
    }

    // syncWorkingDirectory sets the process cwd to this tab's directory so
    // :r/:w relative paths and shell commands match the focused editor.
    func syncWorkingDirectory() {
        guard let c = GoviCwd(handle) else { return }
        let dir = String(cString: c)
        GoviFree(c)
        if !dir.isEmpty {
            FileManager.default.changeCurrentDirectoryPath(dir)
        }
    }

    func updateTitle() {
        var base = documentTitle
        if let c = GoviTitle(handle) {
            let t = String(cString: c)
            GoviFree(c)
            if !t.isEmpty {
                base = t
            }
        }
        guard let w = window else { return }
        // Tab labels use the document name only; dimensions go in the window
        // subtitle (title bar), shared across all tabs in the group.
        w.title = base
        if Settings.showDimensionsInTitle && (w.isKeyWindow || w.tabGroup == nil) {
            w.subtitle = "\(textRows)x\(cols)"
        } else {
            w.subtitle = ""
        }
        updateModeIndicator()
        reconcileTabBar()
    }

    // Mode indicator: a small label at the right end of the title bar showing
    // the editor mode (Command/Insert/Append/Change/Replace, or Ex) and the
    // selection mode (term/gui/hybrid). The title bar always belongs to the
    // visible tab's window, so this is the one place the mode needs to live.
    private lazy var titleModeLabel: NSTextField = {
        let l = NSTextField(labelWithString: "")
        l.font = .systemFont(ofSize: NSFont.smallSystemFontSize)
        l.textColor = .secondaryLabelColor
        return l
    }()

    // Single-tab tab-bar preference (see reconcileTabBar): what the user last
    // chose with Show/Hide Tab Bar while the window had one tab, and the tab
    // count last observed (to tell a closed tab from a user toggle). The
    // "Always show tab bar" setting seeds the preference and is tracked so a
    // checkbox change applies to open windows immediately.
    private var wantsTabBarWhenSingle = Settings.alwaysShowTabBar
    private var lastTabCount = 1
    private var lastAlwaysShowTabBar = Settings.alwaysShowTabBar

    private lazy var titleModeVC: NSTitlebarAccessoryViewController = {
        let vc = NSTitlebarAccessoryViewController()
        let container = NSView()
        container.addSubview(titleModeLabel)
        vc.view = container
        vc.layoutAttribute = .trailing
        return vc
    }()

    private func updateModeIndicator() {
        guard let w = window else { return }
        var mode = bridgeString(GoviModeLabel(handle))
        if GoviExActive(handle) != 0 {
            mode = "Ex"
        }
        let sel: String
        switch selectionMode() {
        case .terminal: sel = "term"
        case .gui: sel = "gui"
        case .contextual: sel = "hybrid"
        }
        let text = "\(mode) \u{00B7} \(sel)"
        if titleModeLabel.stringValue != text {
            titleModeLabel.stringValue = text
            titleModeLabel.sizeToFit()
            let sz = titleModeLabel.frame.size
            titleModeVC.view.setFrameSize(NSSize(width: sz.width + 10, height: sz.height + 6))
            titleModeLabel.setFrameOrigin(NSPoint(x: 0, y: 3))
        }
        // Attach to this view's window, following a tab torn off to a new one.
        if titleModeVC.view.window !== w {
            titleModeVC.removeFromParent() // no-op when not yet attached
            w.addTitlebarAccessoryViewController(titleModeVC)
        }
    }

    // reconcileTabBar applies the "Always show tab bar" setting to a
    // single-tab window (with several tabs the bar is a given). Setting on:
    // the bar is kept visible. Setting off: the bar hides when the window is
    // down to one tab, but Show/Hide Tab Bar still works -- a visibility
    // change seen while the count stays 1 can only be the user's toggle, and
    // it is remembered and restored when a sibling tab later closes (AppKit
    // would leave the bar up).
    private func reconcileTabBar() {
        guard let w = window else { return }
        let always = Settings.alwaysShowTabBar
        let tabCount = w.tabGroup?.windows.count ?? 1
        let barVisible = w.tabGroup?.isTabBarVisible ?? false
        if always != lastAlwaysShowTabBar {
            // The checkbox just changed: adopt it as the single-tab state.
            lastAlwaysShowTabBar = always
            wantsTabBarWhenSingle = always
            if tabCount <= 1 && barVisible != always {
                w.toggleTabBar(nil)
            }
        } else if tabCount <= 1 {
            if always {
                if !barVisible { w.toggleTabBar(nil) }
            } else if lastTabCount > 1 {
                if barVisible != wantsTabBarWhenSingle {
                    w.toggleTabBar(nil)
                }
            } else {
                wantsTabBarWhenSingle = barVisible
            }
        }
        lastTabCount = tabCount
    }

    private func armTimer() {
        timer?.invalidate()
        timer = nil
        let h = handle
        var interval: TimeInterval = 0
        var fire: (() -> Void)?
        if GoviMatchPending(h) != 0 {
            interval = Double(GoviMatchTimeMS(h)) / 1000.0
            fire = { GoviFireTimeout(h) }
        } else if GoviMapPending(h) != 0 {
            interval = 0.5
            fire = { GoviFireTimeout(h) }
        } else if GoviNeedsRecoverySync(h) != 0 {
            interval = 2.0
            fire = { GoviSyncRecovery(h) }
        }
        guard let fire = fire else { return }
        timer = Timer.scheduledTimer(withTimeInterval: interval, repeats: false) { [weak self] _ in
            self?.engineInput(fire)
        }
    }

    // engineInput runs one unit of engine input -- a keystroke, a run of typed
    // scalars, a paste, or a resolved timeout -- on the serial engine queue, then
    // repaints on the main thread. It is the only path that mutates engine state,
    // so engine access stays single-threaded even though the work runs off the
    // main thread.
    //
    // Running it off the main thread is what makes ^C work: a keystroke can launch
    // a long command (a search, :s, :g, or a blocking :! ). On the main thread that
    // would freeze the UI and -- fatally -- stop the ^C meant to abort it from ever
    // being delivered. Here the main thread stays free: a quick command is simply
    // waited for, and a long one is waited for while pumping ONLY key events,
    // forwarding a ^C to GoviInterrupt (safe to call concurrently) so the command
    // aborts. Any other key typed meanwhile is deferred and re-posted so type-ahead
    // is preserved.
    private func engineInput(_ body: @escaping () -> Void) {
        // A pending map/showmatch/recovery timer must not fire and touch the engine
        // on the main thread while the command runs on the queue.
        timer?.invalidate()
        timer = nil

        let done = DispatchSemaphore(value: 0)
        engineQueue.async { body(); done.signal() }

        var deferred: [NSEvent] = []
        // Fast path: most commands finish at once, so don't pump for them.
        if done.wait(timeout: .now() + .milliseconds(20)) == .timedOut {
            // Long command: keep the UI alive for ^C until it finishes.
            while done.wait(timeout: .now()) == .timedOut {
                guard let ev = NSApp.nextEvent(
                    matching: .keyDown, until: Date(timeIntervalSinceNow: 0.02),
                    inMode: .default, dequeue: true) else { continue }
                // nextEvent dequeues app-wide key events; only a ^C aimed at THIS
                // window aborts its command. Anything else (including a ^C meant
                // for another window) is deferred and re-posted below.
                if ev.window === window, Self.isInterruptEvent(ev) {
                    GoviInterrupt(handle) // out of band; aborts the running command
                } else {
                    deferred.append(ev) // type-ahead: replay once the command is done
                }
            }
        }
        step()
        for ev in deferred { NSApp.postEvent(ev, atStart: false) }
    }

    // isInterruptEvent reports whether a key event is the user's ^C: ETX in the
    // characters, or Control-c (mirrors handleControlKey's C0 handling).
    private static func isInterruptEvent(_ ev: NSEvent) -> Bool {
        if let v = ev.characters?.unicodeScalars.first?.value, v == 3 { return true }
        if ev.modifierFlags.intersection(.deviceIndependentFlagsMask).contains(.control),
           let c = ev.charactersIgnoringModifiers, c.lowercased() == "c" {
            return true
        }
        return false
    }

    // MARK: - Mouse selection

    private func cellAt(_ event: NSEvent) -> (x: Int32, y: Int32) {
        let p = convert(event.locationInWindow, from: nil)
        let x = max(0, Int((p.x - padding) / cellW))
        let y = max(0, Int((p.y - padding) / cellH))
        return (Int32(x), Int32(y))
    }

    private func optionRectSelect(_ event: NSEvent) -> Bool {
        event.modifierFlags.intersection(.deviceIndependentFlagsMask).contains(.option)
    }

    private func cellBefore(_ a: ScreenCell, _ b: ScreenCell) -> Bool {
        a.y < b.y || (a.y == b.y && a.x < b.x)
    }

    private func caretBefore(_ a: Caret, _ b: Caret) -> Bool {
        a.line < b.line || (a.line == b.line && a.col < b.col)
    }

    // cellMapsToBuffer reports whether a screen cell sits on editable buffer text.
    private func cellMapsToBuffer(_ c: ScreenCell) -> Bool {
        var line: Int64 = 0, col: Int32 = 0
        return GoviScreenToBuffer(handle, c.x, c.y, &line, &col) != 0
    }

    private func cursorCell() -> ScreenCell { (GoviCursorX(handle), GoviCursorY(handle)) }

    private func editBeep() { NSSound.beep() }

    // moveCursorToCell moves the caret only when the cell is buffer text; a click
    // off the buffer (status/command row, ~, gutter, overlay, ex) leaves it put.
    private func moveCursorToCell(_ c: ScreenCell) {
        var line: Int64 = 0, col: Int32 = 0
        if GoviScreenToBuffer(handle, c.x, c.y, &line, &col) != 0 {
            GoviMoveCursor(handle, line, col)
        }
    }

    private func clearSelection() {
        if !selActive { return }
        selActive = false
        selBlock = false
        selWholeBuffer = false
        selBufBacked = false
        GoviSetSelection(handle, 0, 0, 0, 0, 0)
        GoviSetScreenSelection(handle, 0, 0, 0, 0, 0, 0)
    }

    // setScreenSel stores and highlights a screen selection from a to b (block =
    // Option-drag rectangle, otherwise reading-order linear). a == b is a valid
    // one-cell selection (e.g. a single-character word); callers that mean "no
    // selection" call clearSelection instead.
    private func setScreenSel(_ a: ScreenCell, _ b: ScreenCell, block: Bool) {
        selActive = true
        selBlock = block
        selWholeBuffer = false
        selBufBacked = false // recomputed below; keep editRange on the engine path
        screenSelStart = a
        screenSelEnd = b
        GoviSetScreenSelection(handle, 1, block ? 0 : 1, a.x, a.y, b.x, b.y)
        // Anchor a linear buffer selection in buffer coordinates so its highlight
        // tracks the text on scroll. editRange (engine path) is non-nil exactly
        // when the selection lies entirely on buffer rows.
        if !block, let r = editRange() {
            selBufBacked = true
            selEdit = ((r.0, r.1), (r.2, r.3))
            selDrawA = cellToPos(a)
            selDrawB = cellToPos(b)
        }
    }

    // setBufferAll stores and highlights the editor Select All range in buffer
    // coordinates so it can cover text scrolled off screen.
    private func setBufferAll(_ a: Caret, _ b: Caret) {
        if a.line == b.line && a.col == b.col {
            clearSelection()
            return
        }
        selActive = true
        selBlock = false
        selWholeBuffer = true
        selBufBacked = false
        bufA = a
        bufB = b
        GoviSetSelection(handle, 1, a.line, Int32(a.col), b.line, Int32(b.col))
    }

    // editRange returns the buffer caret range [a, b) for the current selection
    // when it is editable (cut/paste-over/delete/replace-on-type): the whole-
    // buffer Select All, or a linear screen selection lying entirely on buffer
    // text. nil for block selections or any selection touching a non-buffer cell.
    private func editRange() -> (Int64, Int, Int64, Int)? {
        guard selActive else { return nil }
        if selWholeBuffer {
            if caretBefore(bufB, bufA) { return (bufB.line, bufB.col, bufA.line, bufA.col) }
            return (bufA.line, bufA.col, bufB.line, bufB.col)
        }
        if selBlock { return nil }
        // Buffer-anchored: return the stored range, since the on-screen selection
        // cells may have been clamped to the visible region after scrolling.
        if selBufBacked {
            return (selEdit.a.line, selEdit.a.col, selEdit.b.line, selEdit.b.col)
        }
        var l1: Int64 = 0, c1: Int32 = 0, l2: Int64 = 0, c2: Int32 = 0
        if GoviSelectionEditRange(handle, &l1, &c1, &l2, &c2) != 0 {
            return (l1, Int(c1), l2, Int(c2))
        }
        return nil
    }

    private enum SelectionMode: Int32 { case terminal = 0, gui = 1, contextual = 2 }

    private func selectionMode() -> SelectionMode {
        SelectionMode(rawValue: GoviMode(handle)) ?? .contextual
    }

    // selectionCapturesInput reports whether an editing input (a typed rune,
    // paste, Cut, or Backspace/Delete) should act on the current selection rather
    // than pass through to the engine, per selection mode and the editor mode.
    private func selectionCapturesInput() -> Bool {
        guard selActive else { return false }
        switch selectionMode() {
        case .terminal: return false
        case .gui: return true
        case .contextual: return GoviInsertActive(handle) != 0
        }
    }

    // posCell maps a buffer caret to the screen cell it occupies.
    private func posCell(_ line: Int64, _ col: Int) -> ScreenCell {
        var x: Int32 = 0, y: Int32 = 0, vis: Int32 = 0
        GoviPosToCell(handle, line, Int32(col), &x, &y, &vis)
        return (x, y)
    }

    // posCellVis is posCell plus whether the caret is within the laid-out area
    // (false when scrolled off-screen; the cell is then meaningless).
    private func posCellVis(_ p: Caret) -> (cell: ScreenCell, visible: Bool) {
        var x: Int32 = 0, y: Int32 = 0, vis: Int32 = 0
        GoviPosToCell(handle, p.line, Int32(p.col), &x, &y, &vis)
        return ((x, y), vis != 0)
    }

    // cellToPos maps a screen cell to the buffer caret it sits on (clamped into
    // the pane, so a cell past end-of-line yields the end-of-line caret).
    private func cellToPos(_ c: ScreenCell) -> Caret {
        var line: Int64 = 0, col: Int32 = 0
        GoviCellToPos(handle, c.x, c.y, &line, &col)
        return (line, Int(col))
    }

    // activePaneGeom returns the active pane's text-area origin/extent and first
    // visible buffer line, queried live from the engine (the viewport may have
    // just scrolled). nil when there is no active pane (e.g. ex transcript mode).
    private func activePaneGeom() -> (roff: Int32, coff: Int32, rows: Int32, cols: Int32, top: Int64)? {
        let n = GoviPaneCount(handle)
        for i in 0..<n {
            var roff: Int32 = 0, coff: Int32 = 0, prows: Int32 = 0, pcols: Int32 = 0, active: Int32 = 0
            guard GoviPaneGeom(handle, i, &roff, &coff, &prows, &pcols, &active) != 0, active != 0 else {
                continue
            }
            var top: Int64 = 0, lines: Int64 = 0, viewRows: Int32 = 0, scrollable: Int32 = 0
            GoviPaneScrollInfo(handle, i, &top, &lines, &viewRows, &scrollable)
            return (roff, coff, prows, pcols, top)
        }
        return nil
    }

    // reanchorSelection recomputes a buffer-anchored linear selection's screen
    // cells from its buffer endpoints, so the highlight follows the text when the
    // view scrolls. Endpoints scrolled past an edge are clamped to the visible
    // region; a selection scrolled entirely off-screen keeps its buffer anchors
    // (so it reappears on scroll-back) but shows no highlight. Called before every
    // compose; a no-op when nothing has moved.
    private func reanchorSelection() {
        // Skip during an active drag: extendSelection is already setting the cells
        // from the live pointer, and scrolling does not happen mid-drag.
        guard selActive, selBufBacked, !selBlock, !selWholeBuffer, !dragging,
              GoviExActive(handle) == 0, let pane = activePaneGeom() else { return }
        // Reading order on buffer text matches buffer order: lo -> top-left cell,
        // hi -> bottom-right cell.
        var lo = selDrawA, hi = selDrawB
        if caretBefore(hi, lo) { swap(&lo, &hi) }
        let loR = posCellVis(lo), hiR = posCellVis(hi)
        // Entirely off the top (bottom endpoint above the first line) or off the
        // bottom (top endpoint below the last visible line): nothing to draw.
        if (!hiR.visible && hi.line < pane.top) || (!loR.visible && lo.line >= pane.top) {
            screenSelStart = (0, 0)
            screenSelEnd = (0, 0)
            GoviSetScreenSelection(handle, 0, 0, 0, 0, 0, 0)
            return
        }
        // Top endpoint: its cell if visible, else clamped to the top row's first
        // buffer cell (invisible here implies it scrolled above the viewport).
        let loCell: ScreenCell
        if loR.visible {
            loCell = loR.cell
        } else {
            let fx = firstBufferX(onRow: pane.roff)
            loCell = ((fx != nil && fx! >= pane.coff) ? fx! : pane.coff, pane.roff)
        }
        // Bottom endpoint: its cell if visible, else clamped to the pane's
        // bottom-right (invisible here implies it scrolled below the viewport).
        let hiCell: ScreenCell = hiR.visible ? hiR.cell
            : (pane.coff + pane.cols - 1, pane.roff + pane.rows - 1)
        screenSelStart = loCell
        screenSelEnd = hiCell
        GoviSetScreenSelection(handle, 1, 1, loCell.x, loCell.y, hiCell.x, hiCell.y)
    }

    // firstBufferX is the first column on row y that maps to buffer text (just
    // past the line-number gutter), or nil if the row has no buffer text.
    private func firstBufferX(onRow y: Int32) -> Int32? {
        var x: Int32 = 0
        while x < Int32(cols) {
            if cellMapsToBuffer((x, y)) { return x }
            x += 1
        }
        return nil
    }

    private enum ClickClass { case blank, word, punct }

    private func clickClass(_ ch: Character) -> ClickClass {
        if ch == " " || ch == "\t" { return .blank }
        if ch.isLetter || ch.isNumber || ch == "_" { return .word }
        return .punct
    }

    private func wordBounds(in row: String, at col: Int) -> (Int, Int) {
        let chars = Array(row)
        guard !chars.isEmpty else { return (0, 0) }
        var c = col
        if c >= chars.count { c = chars.count - 1 }
        if c < 0 { c = 0 }
        let cls = clickClass(chars[c])
        var start = c, end = c + 1
        while start > 0 && clickClass(chars[start - 1]) == cls { start -= 1 }
        while end < chars.count && clickClass(chars[end]) == cls { end += 1 }
        return (start, end)
    }

    // wordCells returns the inclusive screen-cell span of the word at c: engine
    // (vi) word boundaries on buffer rows, screen-row boundaries elsewhere. nil
    // when c is not on a word (e.g. whitespace).
    private func wordCells(at c: ScreenCell) -> (ScreenCell, ScreenCell)? {
        if cellMapsToBuffer(c) {
            var l1: Int64 = 0, c1: Int32 = 0, l2: Int64 = 0, c2: Int32 = 0
            GoviWordRange(handle, c.x, c.y, &l1, &c1, &l2, &c2)
            if l1 == l2 && c2 <= c1 { return nil }
            return (posCell(l1, Int(c1)), posCell(l2, Int(c2) - 1))
        }
        guard let row = screenRowText(Int(c.y)) else { return nil }
        let (s, e) = wordBounds(in: row, at: Int(c.x))
        if e <= s { return nil }
        return ((Int32(s), c.y), (Int32(e - 1), c.y))
    }

    // lineCells returns the inclusive screen-cell span of the visible row at c,
    // skipping the line-number gutter on buffer rows.
    private func lineCells(at c: ScreenCell) -> (ScreenCell, ScreenCell) {
        let len = screenRowText(Int(c.y))?.count ?? 0
        let startX = firstBufferX(onRow: c.y) ?? 0
        let endX = Int32(max(Int(startX), len - 1))
        return ((startX, c.y), (endX, c.y))
    }

    // extendSelection updates the moving end of the active drag (or shift-extend)
    // to the cell under event, snapped to whole words/lines for word/line
    // granularity, growing in either direction from the originating span. Screen
    // selections never move the cursor.
    private func extendSelection(to event: NSEvent) {
        if selWholeBuffer { clearSelection() } // a drag replaces a Select All
        let c = cellAt(event)
        switch selGranularity {
        case .character:
            // Ignore sub-cell jitter on what is really a click (no selection yet).
            if !selActive && c.x == screenDragAnchor.x && c.y == screenDragAnchor.y { return }
            setScreenSel(screenDragAnchor, c, block: dragBlock)
        case .word:
            let (gs, ge) = wordCells(at: c) ?? (c, c)
            if cellBefore(c, granLo) {
                setScreenSel(granHi, gs, block: dragBlock)
            } else {
                setScreenSel(granLo, ge, block: dragBlock)
            }
        case .line:
            let (gs, ge) = lineCells(at: c)
            if cellBefore(c, granLo) {
                setScreenSel(granHi, gs, block: dragBlock)
            } else {
                setScreenSel(granLo, ge, block: dragBlock)
            }
        }
        step()
    }

    // handleShiftExtendMouseDown extends the selection to a shift-click. With no
    // selection (or after Select All) it anchors a fresh one at the cursor;
    // otherwise it keeps the original anchor (screenDragAnchor / granLo / granHi)
    // so the point where the selection began always stays in the selection across
    // repeated shift-clicks -- only the moving end follows the click.
    private func handleShiftExtendMouseDown(_ event: NSEvent) {
        dragBlock = optionRectSelect(event)
        if !selActive || selWholeBuffer {
            let cur = cursorCell()
            screenDragAnchor = cur
            switch event.clickCount {
            case 2:
                selGranularity = .word
                (granLo, granHi) = wordCells(at: cur) ?? (cur, cur)
            case 3:
                selGranularity = .line
                (granLo, granHi) = lineCells(at: cur)
            default:
                selGranularity = .character
                granLo = cur
                granHi = cur
            }
        }
        dragging = true
        extendSelection(to: event)
    }

    override func mouseDown(with event: NSEvent) {
        window?.makeFirstResponder(self)
        dragBlock = optionRectSelect(event)

        // Shift-click (or shift-click-drag) extends the selection. This must run
        // before the clickCount==2/3 handler: within the system double-click
        // interval AppKit often delivers clickCount=2 on the shift-click even at
        // a different location, which would otherwise replace the selection.
        if shiftDown(event) {
            handleShiftExtendMouseDown(event)
            return
        }

        let c = cellAt(event)

        // Split-pane affordances: grabbing a divider starts a resize drag, and
        // a click in an inactive pane's text focuses that pane first (so the
        // caret/word/line lands in the pane that was clicked). In terminal mode
        // while inserting the mouse is copy-only and must not switch panes,
        // matching the caret exception below.
        var region: Int32 = 0
        let pane = Int(GoviPaneAt(handle, c.x, c.y, &region))
        if region == 2, GoviPaneBelow(handle, Int32(pane)) >= 0, pane < panes.count {
            dividerDrag = (.rows, pane, Int(c.y), panes[pane].rows)
            return
        }
        if region == 3, GoviPaneRight(handle, Int32(pane)) >= 0, pane < panes.count {
            dividerDrag = (.cols, pane, Int(c.x), panes[pane].cols)
            return
        }
        if region == 1, pane < panes.count, !panes[pane].active,
           !(selectionMode() == .terminal && GoviInsertActive(handle) != 0) {
            GoviPaneFocus(handle, Int32(pane))
        }

        if event.clickCount == 2 || event.clickCount == 3 {
            dragging = true
            if event.clickCount == 2 {
                selGranularity = .word
                guard let (ws, we) = wordCells(at: c) else {
                    clearSelection()
                    screenDragAnchor = c
                    granLo = c
                    granHi = c
                    step()
                    return
                }
                granLo = ws
                granHi = we
                screenDragAnchor = ws
                setScreenSel(ws, we, block: dragBlock)
            } else {
                selGranularity = .line
                let (ls, le) = lineCells(at: c)
                granLo = ls
                granHi = le
                screenDragAnchor = ls
                setScreenSel(ls, le, block: dragBlock)
            }
            step()
            return
        }

        // Single click: position the caret on buffer text, no selection yet.
        // Exception: in terminal mode while inserting, the mouse is only for
        // copying, so it must not move the insertion point -- e.g. select+copy
        // elsewhere and paste lands where you were typing, not where you clicked.
        dragging = true
        selGranularity = .character
        clearSelection()
        screenDragAnchor = c
        granLo = c
        granHi = c
        if !(selectionMode() == .terminal && GoviInsertActive(handle) != 0) {
            moveCursorToCell(c)
        }
        step()
    }

    override func mouseDragged(with event: NSEvent) {
        if let drag = dividerDrag {
            dragDivider(drag, to: event)
            return
        }
        guard dragging else { return }
        extendSelection(to: event)
    }

    override func mouseUp(with event: NSEvent) {
        dragging = false
        dividerDrag = nil
    }

    // dragDivider resizes the divider's pane toward "size at mouse-down plus
    // pointer travel". Working from the pane's current size each event keeps
    // the divider stuck at the engine's minimum until the pointer comes back,
    // like a native split view.
    private func dragDivider(_ drag: (kind: DividerKind, pane: Int, startCell: Int, startSize: Int),
                             to event: NSEvent) {
        guard drag.pane < panes.count else { return }
        let p = convert(event.locationInWindow, from: nil)
        let desired: Int
        let current: Int
        switch drag.kind {
        case .rows:
            desired = drag.startSize + Int(floor((p.y - padding) / cellH)) - drag.startCell
            current = panes[drag.pane].rows
        case .cols:
            desired = drag.startSize + Int(floor((p.x - padding) / cellW)) - drag.startCell
            current = panes[drag.pane].cols
        }
        guard desired != current else { return }
        let delta = Int32(desired - current)
        let i = Int32(drag.pane)
        switch drag.kind {
        case .rows: engineInput { GoviDragDividerRows(self.handle, i, delta) }
        case .cols: engineInput { GoviDragDividerCols(self.handle, i, delta) }
        }
    }

    // Wheel / trackpad scrolling moves the viewport like any windowed app; the
    // cursor stays put (it may scroll off-screen). Fractional trackpad deltas
    // accumulate so scrolling is smooth.
    private var scrollAccum: CGFloat = 0

    override func scrollWheel(with event: NSEvent) {
        if event.hasPreciseScrollingDeltas {
            scrollAccum += event.scrollingDeltaY / cellH // points -> lines
        } else {
            scrollAccum += event.scrollingDeltaY // already in lines
        }
        let lines = Int(scrollAccum.rounded(.towardZero))
        guard lines != 0 else { return }
        scrollAccum -= CGFloat(lines)
        // Positive scrollingDeltaY reveals earlier lines (top decreases). The
        // pane under the pointer scrolls, active or not, like any macOS view;
        // in ex (Q) mode there are no panes and the active screen takes it.
        if GoviExActive(handle) != 0 {
            GoviScroll(handle, Int32(-lines))
        } else {
            var region: Int32 = 0
            let pane = GoviPaneAt(handle, cellAt(event).x, cellAt(event).y, &region)
            GoviPaneScrollBy(handle, pane, Int32(-lines))
        }
        step()
    }

    // Hovering a draggable divider shows the matching resize cursor: the
    // status row between stacked panes resizes up/down, the divider column
    // between side-by-side panes resizes left/right.
    override func resetCursorRects() {
        super.resetCursorRects()
        guard panes.count > 1, GoviExActive(handle) == 0 else { return }
        for (i, pane) in panes.enumerated() {
            if GoviPaneBelow(handle, Int32(i)) >= 0 {
                addCursorRect(NSRect(x: padding + CGFloat(pane.coff) * cellW,
                                     y: padding + CGFloat(pane.roff + pane.rows) * cellH,
                                     width: CGFloat(pane.cols) * cellW, height: cellH),
                              cursor: .resizeUpDown)
            }
            if GoviPaneRight(handle, Int32(i)) >= 0 {
                addCursorRect(NSRect(x: padding + CGFloat(pane.coff + pane.cols) * cellW,
                                     y: padding + CGFloat(pane.roff) * cellH,
                                     width: cellW, height: CGFloat(pane.rows + 1) * cellH),
                              cursor: .resizeLeftRight)
            }
        }
    }

    // The tab bar's "+" button is shown by AppKit when this is found in the key
    // window's responder chain; it adds a new tab to this window's group.
    @objc override func newWindowForTab(_ sender: Any?) {
        EditorWindow.openTab(in: window, path: "")
    }

    // MARK: - Clipboard (Edit menu / standard shortcuts)

    // bufferCopyRange is editRange, but for a whole-line (triple-click) buffer
    // selection it extends the end to the next line's start so copy/cut include
    // the trailing newline and a full line round-trips. The last line, which has
    // no following newline, is left as-is.
    private func bufferCopyRange() -> (Int64, Int, Int64, Int)? {
        guard let r = editRange() else { return nil }
        if selGranularity == .line && r.2 < GoviLineCount(handle) {
            return (r.0, r.1, r.2 + 1, 0)
        }
        return r
    }

    @objc func copy(_ sender: Any?) {
        guard selActive else { return }
        let s: String
        if selBlock {
            s = bridgeString(GoviScreenRangeText(handle, screenSelStart.x, screenSelStart.y,
                                                 screenSelEnd.x, screenSelEnd.y))
        } else if let r = bufferCopyRange() {
            // Editable buffer selection: copy buffer text (no line-number gutter).
            s = bridgeString(GoviRangeText(handle, r.0, Int32(r.1), r.2, Int32(r.3)))
        } else {
            // Non-buffer selection (status/overlay/ex/~): copy what is on screen.
            s = bridgeString(GoviScreenLinearRangeText(handle, screenSelStart.x, screenSelStart.y,
                                                       screenSelEnd.x, screenSelEnd.y))
        }
        let pb = NSPasteboard.general
        pb.clearContents()
        pb.setString(s, forType: .string)
    }

    @objc func cut(_ sender: Any?) {
        guard selActive, selectionCapturesInput() else { editBeep(); return }
        guard let r = bufferCopyRange() else { editBeep(); return }
        copy(sender)
        GoviDeleteRange(handle, r.0, Int32(r.1), r.2, Int32(r.3))
        clearSelection()
        step()
    }

    @objc func paste(_ sender: Any?) {
        guard let s = NSPasteboard.general.string(forType: .string) else { return }
        if selActive && selectionCapturesInput() {
            // Replace the selection (wysiwyg, or combined while in insert mode). A
            // bounded edit: run it directly and repaint.
            guard let r = editRange() else { editBeep(); return }
            var buf = Array(s.utf8CString)
            buf.withUnsafeMutableBufferPointer {
                GoviReplaceText(handle, r.0, Int32(r.1), r.2, Int32(r.3), $0.baseAddress)
            }
            clearSelection()
            step()
        } else {
            // Selection is copy-only (or none): drop any highlight and feed the
            // text in the current mode, so a colon-line paste runs as a command,
            // insert mode inserts literally, normal mode interprets it. That command
            // may be long (a pasted :g), so feed it on the interruptible path.
            if selActive { clearSelection() }
            let bytes = Array(s.utf8CString)
            engineInput {
                var b = bytes
                b.withUnsafeMutableBufferPointer { GoviText(self.handle, $0.baseAddress) }
            }
        }
    }

    @objc override func selectAll(_ sender: Any?) {
        if GoviOverlayActive(handle) != 0 || GoviExActive(handle) != 0 {
            let n = Int(GoviRows(handle))
            let c = Int(GoviCols(handle))
            setScreenSel((0, 0), (Int32(max(0, c - 1)), Int32(max(0, n - 1))), block: false)
        } else {
            var line: Int64 = 0, col: Int32 = 0
            GoviEndPos(handle, &line, &col)
            setBufferAll((1, 0), (line, Int(col)))
        }
        step()
    }

    private func bridgeString(_ c: UnsafeMutablePointer<CChar>?) -> String {
        guard let c = c else { return "" }
        defer { GoviFree(c) }
        return String(cString: c)
    }

    // MARK: - Input

    // keyDown splits input two ways. Control-modified keys are vi commands and
    // are translated directly, so Cocoa's built-in Emacs-style key bindings
    // (which would turn ^F/^B/^A/... into cursor motions) never see them.
    // Everything else is handed to the text input system via interpretKeyEvents
    // so plain typing, Option-accents (Option-o -> o-slash), dead keys
    // (Option-u then u -> u-umlaut), and IMEs compose correctly; the composed
    // result arrives back through the NSTextInputClient methods below.
    // Autorepeat (isARepeat) is handled directly: interpretKeyEvents does not
    // deliver repeats to insertText/doCommand.
    override func keyDown(with event: NSEvent) {
        if isControlKey(event) {
            handleControlKey(event)
            return
        }
        if event.isARepeat {
            handleRepeatedKey(event)
            return
        }
        interpretKeyEvents([event])
    }

    // handleRepeatedKey feeds held-key repeats straight to the engine.
    private func handleRepeatedKey(_ event: NSEvent) {
        guard markedText.isEmpty else { return }
        if event.keyCode == 48 {
            dispatchTab()
            return
        }
        if let key = specialKey(for: event) {
            dispatchSpecialKey(key)
            return
        }
        guard let chars = event.characters, !chars.isEmpty else { return }
        if replaceWithText(chars) { return }
        if selActive { clearSelection() } // copy-only selection: drop it, type normally
        let scalars = Array(chars.unicodeScalars)
        engineInput { for s in scalars { GoviKeyRune(self.handle, Int32(s.value), 0) } }
    }

    // specialKey maps an NSEvent key code to an engine special key, if any.
    private func specialKey(for event: NSEvent) -> Int32? {
        switch event.keyCode {
        case 36: return SK.enter
        case 51: return SK.backspace
        case 117: return SK.delete
        case 53: return SK.escape
        case 123: return SK.left
        case 124: return SK.right
        case 125: return SK.down
        case 126: return SK.up
        case 115: return SK.home
        case 119: return SK.end
        case 116: return SK.pageUp
        case 121: return SK.pageDown
        default: return nil
        }
    }

    // dispatchSpecialKey handles a non-text key the same way doCommand would.
    private func dispatchSpecialKey(_ key: Int32) {
        switch key {
        case SK.enter:
            if replaceWithText("\n") { return }
            sendSpecial(SK.enter)
        case SK.backspace, SK.delete:
            if selActive {
                // A selection that captures input is deleted; otherwise the key
                // just clears the highlight (it does not edit or move the cursor).
                if selectionCapturesInput() { deleteSelection() } else { clearSelection(); step() }
                return
            }
            // No selection: the Backspace key is ^? (DEL) -- erases in insert mode,
            // "^? isn't a vi command" in command mode. Forward Delete stays KeyDelete.
            if key == SK.backspace { sendDEL() } else { sendSpecial(key) }
        case SK.escape:
            if selActive { clearSelection() }
            sendSpecial(SK.escape)
        default:
            sendSpecial(key)
        }
    }

    // isControlKey reports whether event is a control-modified keystroke. Use the
    // device-independent modifier mask (not raw modifierFlags) and accept C0 bytes
    // in `characters` — macOS often delivers ^A as SOH without .control set.
    private func isControlKey(_ event: NSEvent) -> Bool {
        let flags = event.modifierFlags.intersection(.deviceIndependentFlagsMask)
        if flags.contains(.control) { return true }
        if let c = event.characters?.unicodeScalars.first, c.value >= 1 && c.value <= 31 {
            return true
        }
        return false
    }

    // handleControlKey feeds vi/ex control bytes to the engine. Colon-line editing
    // expects the C0 code (^A -> SOH), not a letter with a modifier flag.
    private func handleControlKey(_ event: NSEvent) {
        if selActive { clearSelection() } // a command cancels the selection
        var mods: Int32 = 0
        if event.modifierFlags.intersection(.deviceIndependentFlagsMask).contains(.option) {
            mods |= GoviView.modAlt
        }
        let scalars: [Unicode.Scalar]
        if let c = event.characters, let first = c.unicodeScalars.first, first.value >= 1 && first.value <= 31 {
            scalars = Array(c.unicodeScalars)
        } else if let raw = event.charactersIgnoringModifiers {
            scalars = Array(raw.unicodeScalars)
        } else {
            return
        }
        engineInput {
            for scalar in scalars {
                let code = scalar.value <= 31 ? Int32(scalar.value) : Int32(scalar.value & 0x1f)
                GoviKeyRune(self.handle, code, mods)
            }
        }
    }

    // replaceWithText replaces the active selection with s and returns true, or
    // returns false if there was no selection (GUI replace-on-type / paste).
    // replaceWithText replaces the selection with s and returns true when the
    // selection captures the input (wysiwyg, or combined while in insert mode);
    // a captured but non-editable selection beeps and is consumed. Returns false
    // when the selection does not capture input, so the caller passes it through.
    private func replaceWithText(_ s: String) -> Bool {
        guard selActive, selectionCapturesInput() else { return false }
        guard let r = editRange() else { editBeep(); return true }
        var buf = Array(s.utf8CString)
        buf.withUnsafeMutableBufferPointer {
            GoviReplaceType(handle, r.0, Int32(r.1), r.2, Int32(r.3), $0.baseAddress)
        }
        clearSelection()
        step()
        return true
    }

    private func deleteSelection() {
        guard let r = editRange() else { editBeep(); return }
        GoviDeleteRange(handle, r.0, Int32(r.1), r.2, Int32(r.3))
        clearSelection()
        step()
    }

    // MARK: - NSTextInputClient

    func insertText(_ string: Any, replacementRange: NSRange) {
        markedText = ""
        let s = Self.asString(string)
        if s.isEmpty { return }
        if replaceWithText(s) { return } // typed over a selection
        if selActive { clearSelection() } // copy-only selection: drop it, type normally
        let scalars = Array(s.unicodeScalars)
        engineInput { for sc in scalars { GoviKeyRune(self.handle, Int32(sc.value), 0) } }
    }

    override func doCommand(by selector: Selector) {
        switch NSStringFromSelector(selector) {
        case "insertNewline:", "insertLineBreak:", "insertParagraphSeparator:":
            dispatchSpecialKey(SK.enter)
        case "insertTab:":
            dispatchTab()
        case "deleteBackward:":
            dispatchSpecialKey(SK.backspace)
        case "deleteForward:":
            dispatchSpecialKey(SK.delete)
        case "cancelOperation:":
            dispatchSpecialKey(SK.escape)
        case "moveUp:": dispatchSpecialKey(SK.up)
        case "moveDown:": dispatchSpecialKey(SK.down)
        case "moveLeft:": dispatchSpecialKey(SK.left)
        case "moveRight:": dispatchSpecialKey(SK.right)
        case "moveToBeginningOfLine:", "moveToBeginningOfParagraph:":
            dispatchSpecialKey(SK.home)
        case "moveToEndOfLine:", "moveToEndOfParagraph:":
            dispatchSpecialKey(SK.end)
        case "scrollPageUp:", "pageUp:", "pageUpAndModifySelection:":
            dispatchSpecialKey(SK.pageUp)
        case "scrollPageDown:", "pageDown:", "pageDownAndModifySelection:":
            dispatchSpecialKey(SK.pageDown)
        default:
            break // ignore Emacs-style bindings we don't want
        }
    }

    private func dispatchTab() {
        if selActive { clearSelection() }
        engineInput { GoviKeyRune(self.handle, 9, 0) }
    }

    private func sendSpecial(_ key: Int32) {
        if selActive { clearSelection() }
        engineInput { GoviKeySpecial(self.handle, key, 0) }
    }

    // sendDEL feeds ^? (the Backspace key) as a rune so the engine erases in
    // insert/ex mode but reports "^? isn't a vi command" in command mode.
    private func sendDEL() {
        engineInput { GoviKeyRune(self.handle, 0x7f, 0) }
    }

    func setMarkedText(_ string: Any, selectedRange: NSRange, replacementRange: NSRange) {
        markedText = Self.asString(string)
        needsDisplay = true
    }

    func unmarkText() {
        if !markedText.isEmpty {
            markedText = ""
            needsDisplay = true
        }
    }

    func hasMarkedText() -> Bool { !markedText.isEmpty }

    func selectedRange() -> NSRange { NSRange(location: NSNotFound, length: 0) }

    func markedRange() -> NSRange {
        markedText.isEmpty
            ? NSRange(location: NSNotFound, length: 0)
            : NSRange(location: 0, length: markedText.utf16.count)
    }

    func attributedSubstring(forProposedRange range: NSRange, actualRange: NSRangePointer?) -> NSAttributedString? {
        nil
    }

    func validAttributesForMarkedText() -> [NSAttributedString.Key] { [] }

    func characterIndex(for point: NSPoint) -> Int { NSNotFound }

    // firstRect tells the input system where the cursor is (in screen
    // coordinates) so the dead-key/IME candidate window appears there.
    func firstRect(forCharacterRange range: NSRange, actualRange: NSRangePointer?) -> NSRect {
        let r = cellRect(Int(GoviCursorX(handle)), Int(GoviCursorY(handle)))
        let inWindow = convert(r, to: nil)
        return window?.convertToScreen(inWindow) ?? r
    }

    private static func asString(_ v: Any) -> String {
        if let s = v as? String { return s }
        if let a = v as? NSAttributedString { return a.string }
        if let n = v as? NSString { return n as String }
        return ""
    }

    // MARK: - Spell checking

    private func lineText(_ line: Int64) -> String {
        guard let c = GoviLineText(handle, line) else { return "" }
        defer { GoviFree(c) }
        return String(cString: c)
    }

    // updateSpelling recomputes the misspelled words on the visible lines.
    private func updateSpelling() {
        misspellings.removeAll()
        guard spellEnabled else { return }
        // In ex (Q) mode the window shows a transcript, not the buffer, so
        // buffer-anchored squiggles would be misplaced.
        if GoviExActive(handle) != 0 { return }
        let checker = NSSpellChecker.shared
        let count = GoviLineCount(handle)
        var line = GoviTopLine(handle)
        while line <= count {
            var x: Int32 = 0, y: Int32 = 0, vis: Int32 = 0
            GoviPosToCell(handle, line, 0, &x, &y, &vis)
            if vis == 0 { break } // first off-screen line ends the visible range
            for r in misspelledRanges(lineText(line), checker) {
                misspellings.append(Misspelling(line: line, start: r.start, end: r.end))
            }
            line += 1
        }
    }

    private func misspelledRanges(_ text: String, _ checker: NSSpellChecker) -> [RuneRange] {
        if let cached = spellCache[text] { return cached }
        var result: [RuneRange] = []
        let len = (text as NSString).length
        var start = 0
        while start < len {
            let r = checker.checkSpelling(of: text, startingAt: start, language: nil,
                                          wrap: false, inSpellDocumentWithTag: spellTag, wordCount: nil)
            if r.location == NSNotFound || r.length == 0 { break }
            if let rr = runeRange(text, r) { result.append(rr) }
            start = r.location + r.length
        }
        spellCache[text] = result
        if spellCache.count > 4000 { spellCache.removeAll() }
        return result
    }

    // runeRange converts an NSRange (UTF-16) into rune (Unicode scalar) indices,
    // which is what the engine uses for columns.
    private func runeRange(_ s: String, _ ns: NSRange) -> RuneRange? {
        guard let range = Range(ns, in: s) else { return nil }
        let sc = s.unicodeScalars
        return (sc.distance(from: sc.startIndex, to: range.lowerBound),
                sc.distance(from: sc.startIndex, to: range.upperBound))
    }

    private func cellOf(_ line: Int64, _ col: Int) -> (x: Int, y: Int, vis: Bool) {
        var x: Int32 = 0, y: Int32 = 0, v: Int32 = 0
        GoviPosToCell(handle, line, Int32(col), &x, &y, &v)
        return (Int(x), Int(y), v != 0)
    }

    private func drawSpelling() {
        NSColor.systemRed.setStroke()
        for m in misspellings where m.end > m.start {
            let a = cellOf(m.line, m.start)
            let b = cellOf(m.line, m.end - 1)
            if a.vis && b.vis && a.y == b.y {
                squiggle(row: a.y, fromCol: a.x, toCol: b.x + 1)
            } else {
                // Wrapped or partially scrolled word: underline per row.
                var byRow: [Int: (Int, Int)] = [:]
                for r in m.start..<m.end {
                    let c = cellOf(m.line, r)
                    if !c.vis { continue }
                    if let e = byRow[c.y] {
                        byRow[c.y] = (min(e.0, c.x), max(e.1, c.x))
                    } else {
                        byRow[c.y] = (c.x, c.x)
                    }
                }
                for (row, span) in byRow {
                    squiggle(row: row, fromCol: span.0, toCol: span.1 + 1)
                }
            }
        }
    }

    private func squiggle(row: Int, fromCol: Int, toCol: Int) {
        let x0 = padding + CGFloat(fromCol) * cellW
        let x1 = padding + CGFloat(toCol) * cellW
        let yB = padding + CGFloat(row) * cellH + cellH - 1.5
        let path = NSBezierPath()
        let amp: CGFloat = 1.5
        let stepX: CGFloat = 2
        var x = x0
        var up = true
        path.move(to: NSPoint(x: x, y: yB))
        while x < x1 {
            x = min(x + stepX, x1)
            path.line(to: NSPoint(x: x, y: yB - (up ? amp : 0)))
            up.toggle()
        }
        path.lineWidth = 1
        path.stroke()
    }

    // MARK: - Context menu (right-click / control-click)

    private struct LookUpContext { let text: String; let x: Int32; let y: Int32 }

    // menu(for:) is called by AppKit for both right-click and control-click. It
    // builds a standard text context menu: spelling suggestions (when the word
    // is misspelled), a dictionary Look Up, and Cut/Copy/Paste. The word under
    // the click is selected so those commands act on it.
    override func menu(for event: NSEvent) -> NSMenu? {
        window?.makeFirstResponder(self)
        let cell = cellAt(event)
        var line: Int64 = 0, col: Int32 = 0
        GoviCellToPos(handle, cell.x, cell.y, &line, &col)

        let wordText = selectWordForMenu(at: cell) // right-click selects the word

        let menu = NSMenu()
        if let m = misspelling(at: line, col: Int(col)) {
            contextMisspelling = m
            addSuggestions(to: menu, for: m)
        }
        if !wordText.isEmpty {
            let look = menu.addItem(withTitle: "Look Up “\(wordText)”",
                                    action: #selector(lookUp(_:)), keyEquivalent: "")
            look.target = self
            look.representedObject = LookUpContext(text: wordText, x: cell.x, y: cell.y)
            menu.addItem(.separator())
        }
        for (title, sel) in [("Cut", #selector(cut(_:))), ("Copy", #selector(copy(_:))),
                             ("Paste", #selector(paste(_:)))] {
            let item = menu.addItem(withTitle: title, action: sel, keyEquivalent: "")
            item.target = self
        }
        return menu
    }

    // selectWordForMenu selects the word under c (engine word on buffer rows,
    // screen-row word elsewhere) so Cut/Copy act on it, and returns its text for
    // the Look Up item. Empty when the click is not on a word.
    private func selectWordForMenu(at c: ScreenCell) -> String {
        guard let (a, b) = wordCells(at: c) else { return "" }
        selGranularity = .word
        granLo = a
        granHi = b
        screenDragAnchor = a
        setScreenSel(a, b, block: false)
        step()
        if let r = editRange() {
            return bridgeString(GoviRangeText(handle, r.0, Int32(r.1), r.2, Int32(r.3)))
        }
        return bridgeString(GoviScreenLinearRangeText(handle, a.x, a.y, b.x, b.y))
    }

    private func misspelling(at line: Int64, col: Int) -> Misspelling? {
        misspellings.first { $0.line == line && col >= $0.start && col < $0.end }
    }

    private func word(_ m: Misspelling) -> String {
        let sc = Array(lineText(m.line).unicodeScalars)
        guard m.start >= 0, m.end <= sc.count, m.start < m.end else { return "" }
        var v = String.UnicodeScalarView()
        v.append(contentsOf: sc[m.start..<m.end])
        return String(v)
    }

    private func addSuggestions(to menu: NSMenu, for m: Misspelling) {
        let w = word(m)
        let guesses = NSSpellChecker.shared.guesses(
            forWordRange: NSRange(location: 0, length: (w as NSString).length),
            in: w, language: nil, inSpellDocumentWithTag: spellTag) ?? []
        if guesses.isEmpty {
            menu.addItem(withTitle: "No Guesses Found", action: nil, keyEquivalent: "").isEnabled = false
        } else {
            for g in guesses {
                let item = menu.addItem(withTitle: g, action: #selector(replaceSpelling(_:)), keyEquivalent: "")
                item.target = self
                item.representedObject = g
            }
        }
        menu.addItem(.separator())
        let ignore = menu.addItem(withTitle: "Ignore Spelling", action: #selector(ignoreSpelling(_:)), keyEquivalent: "")
        ignore.target = self
        let learn = menu.addItem(withTitle: "Learn Spelling", action: #selector(learnSpelling(_:)), keyEquivalent: "")
        learn.target = self
        menu.addItem(.separator())
    }

    // lookUp shows the system Dictionary popover for the word, anchored at the
    // clicked cell.
    @objc private func lookUp(_ sender: NSMenuItem) {
        guard let ctx = sender.representedObject as? LookUpContext else { return }
        let origin = cellPoint(Int(ctx.x), Int(ctx.y))
        let baseline = NSPoint(x: origin.x, y: origin.y + font.ascender)
        showDefinition(for: NSAttributedString(string: ctx.text), at: baseline)
    }

    @objc private func replaceSpelling(_ sender: NSMenuItem) {
        guard let m = contextMisspelling, let g = sender.representedObject as? String else { return }
        var buf = Array(g.utf8CString)
        buf.withUnsafeMutableBufferPointer {
            GoviReplaceText(handle, m.line, Int32(m.start), m.line, Int32(m.end), $0.baseAddress)
        }
        clearSelection()
        step()
    }

    @objc private func ignoreSpelling(_ sender: Any?) {
        guard let m = contextMisspelling else { return }
        NSSpellChecker.shared.ignoreWord(word(m), inSpellDocumentWithTag: spellTag)
        spellCache.removeAll()
        step()
    }

    @objc private func learnSpelling(_ sender: Any?) {
        guard let m = contextMisspelling else { return }
        NSSpellChecker.shared.learnWord(word(m))
        spellCache.removeAll()
        step()
    }

    // MARK: - Drawing

    override func draw(_ dirtyRect: NSRect) {
        bgColor.setFill()
        bounds.fill()

        let n = Int(GoviRows(handle))

        // Selection highlight: fill the background of reverse-video cells before
        // painting glyphs over them.
        if selActive {
            NSColor.selectedTextBackgroundColor.setFill()
            for y in 0..<n {
                guard let st = rowStyle(y) else { continue }
                for (x, flag) in st.enumerated() where flag == "1" {
                    cellRect(x, y).fill()
                }
            }
        }

        for y in 0..<n {
            drawRow(y)
        }

        drawPaneChrome()

        if !misspellings.isEmpty {
            drawSpelling()
        }

        // Marked (uncommitted) text from a dead key/IME: draw it underlined at
        // the cursor and skip the block cursor while it is pending.
        if !markedText.isEmpty {
            let cx = Int(GoviCursorX(handle))
            let cy = Int(GoviCursorY(handle))
            let attrs: [NSAttributedString.Key: Any] = [
                .font: font, .foregroundColor: fgColor,
                .underlineStyle: NSUnderlineStyle.single.rawValue,
            ]
            for (i, ch) in markedText.enumerated() {
                let r = cellRect(cx + i, cy)
                bgColor.setFill()
                r.fill()
                (String(ch) as NSString).draw(at: r.origin, withAttributes: attrs)
            }
        } else if GoviCursorVisible(handle) != 0 {
            drawCursor(col: Int(GoviCursorX(handle)), row: Int(GoviCursorY(handle)))
        }
    }

    // drawCursor renders the editor cursor per Settings.cursorStyle. A box is a
    // filled cell (with the glyph inverted) when the window is focused, and a
    // hollow outline when it is not; a bar is a thin rule at the insertion point.
    private func drawCursor(col: Int, row: Int) {
        let rect = cellRect(col, row)
        let focused = window?.isKeyWindow ?? false
        cursorColor.set()
        switch Settings.cursorStyle {
        case .bar:
            var bar = rect
            bar.size.width = max(1, min(2, cellW))
            bar.fill()
        case .box:
            if focused {
                rect.fill()
                if let ch = charAt(col, row), ch != " " {
                    drawChar(ch, col: col, row: row, color: bgColor)
                }
            } else {
                let outline = NSBezierPath(rect: rect.insetBy(dx: 0.5, dy: 0.5))
                outline.lineWidth = 1
                outline.stroke()
            }
        }
    }

    // drawPaneChrome frames split panes as subwindows: the '|' character
    // column between vertical splits is repainted as a native hairline
    // divider, each pane (text plus its status line) gets a 1px border, and
    // inactive panes are washed toward the background to read as unfocused.
    private func drawPaneChrome() {
        guard panes.count > 1, GoviExActive(handle) == 0 else { return }
        let sep = NSColor.separatorColor
        for pane in panes {
            let frame = NSRect(x: padding + CGFloat(pane.coff) * cellW,
                               y: padding + CGFloat(pane.roff) * cellH,
                               width: CGFloat(pane.cols) * cellW,
                               height: CGFloat(pane.rows + 1) * cellH)
            if pane.coff + pane.cols < cols {
                // The sacrificed divider column: erase the composed '|' glyphs
                // and draw a centered hairline instead.
                let strip = NSRect(x: frame.maxX, y: frame.minY, width: cellW, height: frame.height)
                bgColor.setFill()
                strip.fill()
                sep.setFill()
                NSRect(x: strip.midX - 0.5, y: strip.minY, width: 1, height: strip.height).fill()
            }
            sep.setStroke()
            let border = NSBezierPath(rect: frame.insetBy(dx: 0.5, dy: 0.5))
            border.lineWidth = 1
            border.stroke()
            if !pane.active {
                bgColor.withAlphaComponent(0.35).setFill()
                frame.fill()
            }
        }
    }

    private func drawRow(_ y: Int) {
        guard let s = screenRowText(y) else { return }
        for (i, ch) in s.enumerated() where ch != " " {
            drawChar(ch, col: i, row: y, color: fgColor)
        }
    }

    private func drawChar(_ ch: Character, col: Int, row: Int, color: NSColor) {
        let attrs: [NSAttributedString.Key: Any] = [.font: font, .foregroundColor: color]
        (String(ch) as NSString).draw(at: cellPoint(col, row), withAttributes: attrs)
    }

    private func screenRowText(_ y: Int) -> String? {
        guard let c = GoviRowText(handle, Int32(y)) else { return nil }
        defer { GoviFree(c) }
        return String(cString: c)
    }

    private func rowStyle(_ y: Int) -> String? {
        guard let c = GoviRowStyle(handle, Int32(y)) else { return nil }
        defer { GoviFree(c) }
        return String(cString: c)
    }

    private func charAt(_ col: Int, _ row: Int) -> Character? {
        guard let s = screenRowText(row) else { return nil }
        let arr = Array(s)
        return col < arr.count ? arr[col] : " "
    }
}
