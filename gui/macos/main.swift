import Carbon
import Cocoa
import Darwin

// GoVi: a native macOS application with the govi (Go nvi) editor engine embedded
// in-process. The engine is linked in as a C archive (libgovi); this app drives
// it and renders its screen in a custom NSView. nvi is *embedded*, not exec'd.
//
// The app is multi-window: each EditorWindow owns its own engine instance
// (a libgovi handle). Settings.openFilesIn controls whether opened files start
// in a new window or as a tab in the frontmost window.

// EditorWindow owns one window, its GoviView, and the embedded engine behind it.
// Instances keep themselves alive in a static set until their window closes.
final class EditorWindow: NSObject, NSWindowDelegate {
    private static var windows: Set<EditorWindow> = []
    private static var cascadePoint = NSPoint.zero

    let window: NSWindow
    let view: GoviView
    var path: String // the file this window is editing ("" = untitled)
    private let tempFileToDelete: String? // nvi-style vi.XXXXXX temp, removed on close

    // anyOpen reports whether any editor window exists.
    static var anyOpen: Bool { !windows.isEmpty }

    // anyEditing reports whether a window/tab is editing path.
    static func anyEditing(path: String) -> Bool {
        let p = LaunchPath.normalize(path)
        return windows.contains { LaunchPath.normalize($0.path) == p }
    }

    // keyEditor returns the editor owning the key window, if any.
    static func keyEditor() -> EditorWindow? {
        guard let view = NSApp.keyWindow?.contentView as? GoviView else { return nil }
        return windows.first { $0.view === view }
    }

    // make creates an editor for path (empty path = an empty buffer) without
    // presenting it. Returns nil if the file could not be opened.
    private static func make(path: String, cwd: String = "", silent: Bool = false) -> EditorWindow? {
        let fg = Settings.defaultForegroundColorSpec
        let bg = Settings.defaultBackgroundColorSpec
        // Resolve the working directory before starting the engine so LoadStartup
        // reads ./.nexrc / .exrc from the right place. A fileless window (Finder
        // launch, File > New) has no launch-payload cwd, so use the configured
        // initial directory (home by default) instead of the process cwd ("/").
        var dir = cwd
        if dir.isEmpty && path.isEmpty {
            dir = Settings.resolvedInitialDirectory
        }
        let handle = path.withCString { pathPtr in
            fg.withCString { fgPtr in
                bg.withCString { bgPtr in
                    dir.withCString { cwdPtr in
                        GoviStart(UnsafeMutablePointer(mutating: pathPtr),
                                  UnsafeMutablePointer(mutating: fgPtr),
                                  UnsafeMutablePointer(mutating: bgPtr),
                                  UnsafeMutablePointer(mutating: cwdPtr),
                                  silent ? 1 : 0)
                    }
                }
            }
        }
        guard handle != 0 else {
            let alert = NSAlert()
            alert.messageText = "Could not open “\(path)”."
            alert.runModal()
            return nil
        }
        if LaunchPath.isGoviTempFile(LaunchPath.normalize(path)) {
            GoviSetTemporary(handle)
        }
        let w = EditorWindow(handle: handle, path: path)
        windows.insert(w)
        return w
    }

    // open presents path in a new standalone window.
    @discardableResult
    static func open(path: String, cwd: String = "", silent: Bool = false) -> EditorWindow? {
        guard let w = make(path: path, cwd: cwd, silent: silent) else { return nil }
        w.showStandalone()
        return w
    }

    // scratchPath creates an empty $TMPDIR/vi.XXXXXX file and returns its path so
    // a no-file window edits a throwaway temp like nvi and `govi -g`. make()
    // detects it (isGoviTempFile), marks it temporary, and deletes it on close.
    // Returns "" if the temp file cannot be created (open then falls back to an
    // unnamed buffer).
    private static func scratchPath() -> String {
        let template = (NSTemporaryDirectory() as NSString).appendingPathComponent("vi.XXXXXX")
        var buf = Array(template.utf8CString)
        let fd = mkstemp(&buf)
        if fd < 0 { return "" }
        close(fd)
        return String(cString: buf)
    }

    // openScratch opens a new standalone window on a throwaway temp buffer, with
    // the configured initial directory as its working dir (the temp lives in
    // $TMPDIR, but :e/:r/:! should resolve from the user's chosen directory).
    @discardableResult
    static func openScratch() -> EditorWindow? {
        open(path: scratchPath(), cwd: Settings.resolvedInitialDirectory)
    }

    // openScratchTab opens a throwaway temp buffer as a tab in keyWindow's group.
    static func openScratchTab(in keyWindow: NSWindow?) {
        openTab(in: keyWindow, path: scratchPath(), cwd: Settings.resolvedInitialDirectory)
    }

    // existing returns the window already editing path, if any.
    private static func existing(path p: String) -> EditorWindow? {
        p.isEmpty ? nil : windows.first(where: { $0.path == p })
    }

    // openPaths opens one or more files according to Settings.openFilesIn. In tab
    // mode, files join the frontmost window's tab group (or start a new window
    // when none exists). In new-window mode, each file gets its own window.
    // Already-open files are focused in place rather than duplicated.
    static func openPaths(_ paths: [String], cwd: String = "", fifo: String? = nil, silent: Bool = false) {
        let normalized = paths.map { LaunchPath.normalize($0) }
        guard !normalized.isEmpty else { return }
        WaitCoordinator.shared.registerWait(paths: normalized, fifo: fifo)

        // With tabbing off, force separate windows regardless of the popup.
        let mode: Settings.OpenFilesIn = Settings.useTabs ? Settings.openFilesIn : .newWindow
        switch mode {
        case .newWindow:
            for p in normalized {
                if let w = existing(path: p) {
                    w.window.makeKeyAndOrderFront(nil)
                } else {
                    _ = open(path: p, cwd: cwd, silent: silent)
                }
            }
        case .tab:
            openAsTabs(normalized, cwd: cwd, silent: silent)
        }
        WaitCoordinator.shared.checkComplete()
    }

    // openAsTabs opens paths in the frontmost window's tab group. The first file
    // starts a new window when no anchor exists yet.
    private static func openAsTabs(_ paths: [String], cwd: String = "", silent: Bool = false) {
        var anchor = NSApp.keyWindow ?? windows.first?.window
        for p in paths {
            if let w = existing(path: p) {
                w.window.makeKeyAndOrderFront(nil)
                anchor = w.window
                continue
            }
            if let a = anchor {
                openTab(in: a, path: p, cwd: cwd, silent: silent)
            } else if let w = open(path: p, cwd: cwd, silent: silent) {
                anchor = w.window
            }
        }
    }

    // openTab presents path as a new tab in keyWindow's tab group, or as a
    // standalone window if there is no key window to tab into.
    static func openTab(in keyWindow: NSWindow?, path: String, cwd: String = "", silent: Bool = false) {
        guard let w = make(path: path, cwd: cwd, silent: silent) else { return }
        if let key = keyWindow, key !== w.window {
            key.addTabbedWindow(w.window, ordered: .above)
            w.window.makeKeyAndOrderFront(nil)
            w.window.makeFirstResponder(w.view)
            w.view.updateGeometry()
            w.view.updateTitle()
        } else {
            w.showStandalone()
        }
    }

    private init(handle: Int64, path: String) {
        self.path = LaunchPath.normalize(path)
        self.tempFileToDelete = LaunchPath.isGoviTempFile(self.path) ? self.path : nil
        let size = GoviView.contentSize(
            textRows: Settings.defaultTextRows, cols: Settings.defaultColumns)
        let frame = NSRect(x: 0, y: 0, width: size.width, height: size.height)
        window = NSWindow(
            contentRect: frame,
            styleMask: [.titled, .closable, .miniaturizable, .resizable],
            backing: .buffered, defer: false)
        view = GoviView(frame: frame, handle: handle)
        super.init()
        view.documentTitle = LaunchPath.displayTitle(for: self.path)
        window.contentView = view
        window.delegate = self
        window.isReleasedWhenClosed = false
        // Native macOS tabbing: a shared identifier lets these windows be tabbed
        // together and dragged between windows; .automatic respects the user's
        // "prefer tabs" setting for Cmd-N while still allowing explicit tabs.
        window.tabbingIdentifier = "govi"
        // .automatic respects the system "prefer tabs" setting; .disallowed gives
        // standalone windows with no tab bar (Settings: "Use window tabs" off).
        window.tabbingMode = Settings.useTabs ? .automatic : .disallowed
    }

    private func showStandalone() {
        // Start cascading from the top-left of the main screen's visible area
        // (below the menu bar), not the bottom-left corner of the display.
        if EditorWindow.cascadePoint == .zero, let vf = NSScreen.main?.visibleFrame {
            EditorWindow.cascadePoint = NSPoint(x: vf.minX + 20, y: vf.maxY - 20)
        }
        EditorWindow.cascadePoint = window.cascadeTopLeft(from: EditorWindow.cascadePoint)
        window.makeKeyAndOrderFront(nil)
        window.makeFirstResponder(view)
        fitWindowToGrid()
        view.updateGeometry()
        view.updateTitle()
        // The macOS tab bar (with "prefer tabs" on) appears as the window is
        // ordered front and steals editor rows; re-fit on the next tick once it
        // has been laid out.
        DispatchQueue.main.async { [weak self] in self?.fitWindowToGrid() }
    }

    // fitWindowToGrid grows/shrinks the window so the editor's content area is
    // exactly the configured rows x cols, compensating for chrome that steals
    // space after the window is shown -- notably the macOS tab bar. The top edge
    // stays fixed.
    private func fitWindowToGrid() {
        window.layoutIfNeeded()
        let desired = GoviView.contentSize(
            textRows: Settings.defaultTextRows, cols: Settings.defaultColumns)
        let deltaH = desired.height - view.bounds.height
        let deltaW = desired.width - view.bounds.width
        if abs(deltaH) < 0.5 && abs(deltaW) < 0.5 { return }
        var f = window.frame
        f.origin.y -= deltaH // keep the top edge fixed as the window grows down
        f.size.width += deltaW
        f.size.height += deltaH
        // Growing downward can push the window off the bottom of the screen; keep
        // it within the visible frame.
        if let vf = (window.screen ?? NSScreen.main)?.visibleFrame {
            if f.maxY > vf.maxY { f.origin.y = vf.maxY - f.size.height }
            if f.minY < vf.minY { f.origin.y = vf.minY }
            if f.maxX > vf.maxX { f.origin.x = vf.maxX - f.size.width }
            if f.minX < vf.minX { f.origin.x = vf.minX }
        }
        window.setFrame(f, display: true)
    }

    func windowDidBecomeKey(_ notification: Notification) {
        view.syncWorkingDirectory()
        view.updateTitle()
        view.needsDisplay = true // refill the cursor (hollow box -> filled)
    }

    func windowDidResignKey(_ notification: Notification) {
        view.needsDisplay = true // hollow the box cursor while unfocused
    }

    func windowShouldClose(_ sender: NSWindow) -> Bool {
        confirmClose()
    }

    func windowWillClose(_ notification: Notification) {
        let closedPath = path
        GoviClose(view.handle)
        EditorWindow.windows.remove(self)
        WaitCoordinator.shared.editorClosed(path: closedPath)
        if let tmp = tempFileToDelete {
            try? FileManager.default.removeItem(atPath: tmp)
        }
    }

    // confirmQuit prompts for every modified window before the app terminates
    // (Cmd-Q), returning false if the user cancels any prompt. Mirrors :q's
    // unsaved-changes check, which Cmd-Q would otherwise bypass.
    static func confirmQuit() -> Bool {
        for w in windows where GoviModified(w.view.handle) != 0 || GoviTempPending(w.view.handle) != 0 {
            w.window.makeKeyAndOrderFront(nil) // show which document is prompting
            if !w.confirmClose() { return false }
        }
        return true
    }

    // confirmClose returns true when the window/tab may close. When
    // Settings.warnOnUnsavedClose is set and the buffer is modified, the user
    // is prompted to save, discard, or cancel.
    private func confirmClose() -> Bool {
        // A temp buffer with content warns even when "unmodified" (after :w), since
        // closing still discards the throwaway -- like :q.
        if !Settings.warnOnUnsavedClose
            || (GoviModified(view.handle) == 0 && GoviTempPending(view.handle) == 0) {
            return true
        }

        let alert = NSAlert()
        alert.alertStyle = .warning
        if GoviIsTemporary(view.handle) != 0 {
            // A govi -g temp buffer has no real file name; writing it is pointless
            // (it is deleted on close), so make the discard explicit and offer
            // Save As rather than a misleading plain Save.
            alert.messageText = "This is a temporary buffer with no file name."
            alert.informativeText = "Its contents will be discarded when it closes. Save them to a file?"
            alert.addButton(withTitle: "Save As…")
            alert.addButton(withTitle: "Discard")
            alert.addButton(withTitle: "Cancel")
        } else {
            let displayName = path.isEmpty ? "Untitled" : (path as NSString).lastPathComponent
            alert.messageText = "Do you want to save the changes made to “\(displayName)”?"
            alert.informativeText = "Your changes will be lost if you don't save them."
            alert.addButton(withTitle: "Save")
            alert.addButton(withTitle: "Don't Save")
            alert.addButton(withTitle: "Cancel")
        }

        switch alert.runModal() {
        case .alertFirstButtonReturn:
            return saveForClose()
        case .alertSecondButtonReturn:
            return true
        default:
            GoviClearQuit(view.handle)
            return false
        }
    }

    // save writes the buffer with :w (current file). Returns false on error.
    @discardableResult
    func save() -> Bool {
        if GoviSave(view.handle, nil) != 0 {
            let alert = NSAlert()
            alert.messageText = "The document could not be saved."
            if path.isEmpty {
                alert.informativeText = "No file name."
            } else {
                alert.informativeText = "“\(path)” could not be written."
            }
            alert.alertStyle = .warning
            alert.runModal()
            return false
        }
        view.updateTitle()
        return true
    }

    // saveAs prompts for a destination and saves there (:f name then :w).
    // Cancel is a no-op. Returns false on error or cancel.
    @discardableResult
    func saveAs() -> Bool {
        let panel = savePanel()
        guard panel.runModal() == .OK, let url = panel.url else { return false }
        return writeBuffer(path: url.path)
    }

    private func savePanel() -> NSSavePanel {
        let panel = NSSavePanel()
        panel.canCreateDirectories = true
        if let c = GoviCwd(view.handle) {
            let dir = String(cString: c)
            GoviFree(c)
            if !dir.isEmpty {
                panel.directoryURL = URL(fileURLWithPath: dir)
            }
        }
        panel.nameFieldStringValue = LaunchPath.displayTitle(for: path)
        return panel
    }

    // writeBuffer saves to path via GoviSaveAs (:f then :w).
    @discardableResult
    private func writeBuffer(path target: String) -> Bool {
        let std = (target as NSString).standardizingPath
        let saved = std.withCString { ptr in
            GoviSaveAs(view.handle, UnsafeMutablePointer(mutating: ptr)) == 0
        }
        if !saved {
            let alert = NSAlert()
            alert.messageText = "The document could not be saved."
            if std.isEmpty {
                alert.informativeText = "No file name."
            } else {
                alert.informativeText = "“\(std)” could not be written."
            }
            alert.alertStyle = .warning
            alert.runModal()
            return false
        }
        path = std
        view.documentTitle = LaunchPath.displayTitle(for: path)
        view.updateTitle()
        return true
    }

    private func saveForClose() -> Bool {
        var target = path
        let rename = target.isEmpty || GoviIsTemporary(view.handle) != 0
        if rename {
            let panel = savePanel()
            guard panel.runModal() == .OK, let url = panel.url else {
                GoviClearQuit(view.handle)
                return false
            }
            target = url.path
        }
        let ok = rename ? writeBuffer(path: target) : save()
        if !ok {
            GoviClearQuit(view.handle)
            return false
        }
        return true
    }
}

// LaunchPath reads the validated, one-shot govi:// launch payload and normalizes
// file paths from it and from macOS open-documents (Finder) events.
enum LaunchPath {
    private static var supportDir: URL {
        FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("GoVi", isDirectory: true)
    }

    // launchDir is the fixed directory the launcher drops govi:// payloads in.
    // The app only ever reads payloads from here, so a crafted govi:// URL can't
    // make it read an arbitrary file.
    static var launchDirURL: URL {
        supportDir.appendingPathComponent("launch", isDirectory: true)
    }

    struct LaunchPayload {
        var cwd = ""
        var silent = false
        var files: [String] = []
        var fifo: String?
    }

    // consumePayload validates a govi:// token, reads the one-shot payload from
    // the fixed launch dir, deletes it, and returns its data. The token must be a
    // bare "ctx-..." name (no path separators) resolving to a regular file
    // directly inside launchDir; anything else is rejected. The payload is pure
    // data -- the app only opens the listed files and never writes or executes
    // anything from it.
    // pendingPayloadTokens lists payloads the launcher left in the fixed dir.
    // Used on cold launch, where the open/URL Apple Event is unreliable but the
    // payload is already on disk (the launcher writes it before `open`).
    static func pendingPayloadTokens() -> [String] {
        guard let names = try? FileManager.default.contentsOfDirectory(atPath: launchDirURL.path)
        else { return [] }
        return names.filter { $0.hasPrefix("ctx-") }
    }

    static func consumePayload(token: String) -> LaunchPayload? {
        guard token.range(of: "^ctx-[A-Za-z0-9]+$", options: .regularExpression) != nil else {
            return nil
        }
        let url = launchDirURL.appendingPathComponent(token, isDirectory: false)
        guard url.deletingLastPathComponent().standardizedFileURL.path
                == launchDirURL.standardizedFileURL.path,
              let text = try? String(contentsOf: url, encoding: .utf8) else {
            return nil
        }
        try? FileManager.default.removeItem(at: url) // one-shot
        var p = LaunchPayload()
        for line in text.split(whereSeparator: \.isNewline) {
            guard let eq = line.firstIndex(of: "=") else { continue }
            let key = String(line[..<eq])
            let val = String(line[line.index(after: eq)...])
            switch key {
            case "cwd": p.cwd = val
            case "silent": p.silent = (val == "1")
            case "file": p.files.append(normalize(val))
            case "fifo": p.fifo = val
            default: break
            }
        }
        return p
    }

    // isGoviTempFile reports whether path is a temp file `govi -g` (no files)
    // created for an empty editor (nvi-style vi.XXXXXX in the temp dir). Such a
    // file is deleted when its window/tab closes.
    static func isGoviTempFile(_ path: String) -> Bool {
        guard (path as NSString).lastPathComponent.hasPrefix("vi.") else { return false }
        let parent = URL(fileURLWithPath: (path as NSString).deletingLastPathComponent).standardizedFileURL.path
        return parent == URL(fileURLWithPath: NSTemporaryDirectory()).standardizedFileURL.path
    }

    // displayTitle is the name shown in the window title bar and tab label. Temp
    // buffers keep their vi.XXXXXX path in the engine (status line, :f) but read
    // as "Untitled" in chrome.
    static func displayTitle(for path: String) -> String {
        if path.isEmpty || isGoviTempFile(normalize(path)) { return "Untitled" }
        return (path as NSString).lastPathComponent
    }

    static func normalize(_ path: String) -> String {
        if path.isEmpty { return "" } // no file: stay empty, don't become the cwd
        let p = (path as NSString).standardizingPath
        if (p as NSString).isAbsolutePath { return p }
        let procCwd = FileManager.default.currentDirectoryPath
        if !procCwd.isEmpty {
            return (procCwd as NSString).appendingPathComponent(p)
        }
        return p
    }

    static func pathFromOpenURL(_ url: URL) -> String {
        normalize(url.standardizedFileURL.path)
    }
}

// WaitCoordinator unblocks `govi -g -w` launchers. Each -w invocation records a
// FIFO in launch-wait and is tracked as an independent session over only the
// files that invocation opened; its FIFO is signaled (opened for writing) once
// none of those files remain open in any window/tab. Later invocations -- with
// or without -w -- start their own session (or none) and never extend an
// existing one, so closing just the -w file releases its launcher.
final class WaitCoordinator {
    static let shared = WaitCoordinator()

    private struct Session {
        let fifo: String
        let paths: Set<String>
    }
    private var sessions: [Session] = []
    private let lock = NSLock()

    // registerWait starts a wait session when the just-launched invocation
    // recorded a FIFO in launch-wait; paths are the files it opened. Invocations
    // without -w leave no launch-wait and add no session. It is called before the
    // windows exist, so completion is first evaluated by the checkComplete() that
    // openPaths runs after opening them.
    func registerWait(paths: [String], fifo: String?) {
        lock.lock()
        defer { lock.unlock() }
        guard let fifo = fifo, !fifo.isEmpty else { return }
        sessions.append(Session(fifo: fifo, paths: Set(paths)))
    }

    func editorClosed(path: String) {
        lock.lock()
        defer { lock.unlock() }
        checkLocked()
    }

    func checkComplete() {
        lock.lock()
        defer { lock.unlock() }
        checkLocked()
    }

    // checkLocked signals and drops every session whose files are all closed.
    private func checkLocked() {
        sessions = sessions.filter { session in
            if session.paths.contains(where: { EditorWindow.anyEditing(path: $0) }) {
                return true // still waiting on at least one open file
            }
            signal(fifo: session.fifo)
            return false
        }
    }

    private func signal(fifo: String) {
        DispatchQueue.global(qos: .utility).async {
            // Only ever signal a real FIFO: never open/write/truncate a regular
            // file, even though the path comes from a fixed-location payload.
            var st = stat()
            guard stat(fifo, &st) == 0, (st.st_mode & S_IFMT) == S_IFIFO else { return }
            let fd = open(fifo, O_WRONLY)
            if fd >= 0 {
                var byte: UInt8 = 0
                _ = write(fd, &byte, 1)
                close(fd)
            }
            unlink(fifo)
        }
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate {
    // Queued during a cold launch (handled in finishColdLaunch): file URLs from
    // Finder, and govi:// launch tokens from the `govi -g` launcher.
    private var pendingOpenPaths: [String] = []
    private var pendingLaunchTokens: [String] = []
    private var coldLaunchComplete = false

    // launchToken extracts the ctx token from a govi://open?ctx=... URL.
    private func launchToken(from url: URL) -> String {
        guard let comps = URLComponents(url: url, resolvingAgainstBaseURL: false),
              let val = comps.queryItems?.first(where: { $0.name == "ctx" })?.value
        else { return "" }
        return val
    }

    // handleLaunch reads a validated one-shot payload and opens its files with the
    // launcher's cwd (and -w FIFO). Data only: it never writes or runs anything.
    private func handleLaunch(_ token: String) {
        guard let p = LaunchPath.consumePayload(token: token), !p.files.isEmpty else { return }
        EditorWindow.openPaths(p.files, cwd: p.cwd, fifo: p.fifo, silent: p.silent)
    }

    func applicationWillFinishLaunching(_ notification: Notification) {
        // Custom URL schemes (govi://) are delivered as a kAEGetURL Apple Event,
        // not via application(_:open:). Register early so a cold-launch URL is
        // caught.
        NSAppleEventManager.shared().setEventHandler(
            self,
            andSelector: #selector(handleGetURL(_:withReplyEvent:)),
            forEventClass: AEEventClass(kInternetEventClass),
            andEventID: AEEventID(kAEGetURL))
    }

    @objc func handleGetURL(_ event: NSAppleEventDescriptor, withReplyEvent: NSAppleEventDescriptor) {
        guard let s = event.paramDescriptor(forKeyword: AEKeyword(keyDirectObject))?.stringValue,
              let url = URL(string: s), url.scheme == "govi" else { return }
        let token = launchToken(from: url)
        if coldLaunchComplete { handleLaunch(token) } else { pendingLaunchTokens.append(token) }
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        // Files passed by direct exec (GoVi.app/Contents/MacOS/GoVi file ...).
        let cwd = FileManager.default.currentDirectoryPath
        let paths = CommandLine.arguments.dropFirst().map { arg -> String in
            LaunchPath.normalize((arg as NSString).isAbsolutePath ? arg : "\(cwd)/\(arg)")
        }
        if !paths.isEmpty {
            EditorWindow.openPaths(Array(paths))
        }
        // Defer twice so launch-files is written and any open event is queued
        // before we create an empty window.
        DispatchQueue.main.async {
            DispatchQueue.main.async {
                self.finishColdLaunch()
            }
        }
        NSApp.activate(ignoringOtherApps: true)
    }

    private func finishColdLaunch() {
        // Cold-launch open/URL Apple Events are unreliable, so besides any token
        // delivered via application(_:open:), pick up the payload the launcher
        // already wrote to disk. consumePayload is one-shot, so a token seen both
        // ways still opens exactly once.
        var tokens = pendingLaunchTokens
        pendingLaunchTokens.removeAll()
        tokens.append(contentsOf: LaunchPath.pendingPayloadTokens())
        for token in tokens { handleLaunch(token) }
        if !pendingOpenPaths.isEmpty {
            EditorWindow.openPaths(pendingOpenPaths)
        }
        pendingOpenPaths.removeAll()
        coldLaunchComplete = true
        if !EditorWindow.anyOpen {
            EditorWindow.openScratch()
        }
    }

    // applicationShouldHandleReopen fires when `open`-ing an already-running app
    // (and on Dock clicks). `govi -g` with no files leaves a sentinel so it opens
    // a fresh empty editor even when windows are already open; a plain reopen with
    // no windows opens one too.
    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows: Bool) -> Bool {
        if !hasVisibleWindows {
            EditorWindow.openScratch()
        }
        return true
    }

    // application(_:open:) receives both govi://open?ctx=... launch URLs (from the
    // `govi -g` launcher; routed here by LaunchServices, cold or already running)
    // and plain file URLs (Finder double-clicks / drags). Launch URLs carry their
    // context in a validated one-shot payload; file URLs open with the default cwd.
    func application(_ application: NSApplication, open urls: [URL]) {
        var filePaths: [String] = []
        for url in urls {
            if url.scheme == "govi" {
                let token = launchToken(from: url)
                if coldLaunchComplete { handleLaunch(token) } else { pendingLaunchTokens.append(token) }
            } else if url.isFileURL {
                filePaths.append(LaunchPath.pathFromOpenURL(url))
            }
        }
        if !filePaths.isEmpty {
            if coldLaunchComplete { EditorWindow.openPaths(filePaths) } else { pendingOpenPaths.append(contentsOf: filePaths) }
        }
        NSApp.activate(ignoringOtherApps: true)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }

    // applicationShouldTerminate gives Cmd-Q the same unsaved-changes check as
    // :q / closing a window, instead of quitting and losing modified buffers.
    func applicationShouldTerminate(_ sender: NSApplication) -> NSApplication.TerminateReply {
        EditorWindow.confirmQuit() ? .terminateNow : .terminateCancel
    }

    // File > New: a new window on a throwaway temp buffer (like nvi / govi -g).
    @objc func newWindow(_ sender: Any?) {
        EditorWindow.openScratch()
    }

    // File > New Tab: a throwaway temp buffer as a tab in the current window's group.
    @objc func newTab(_ sender: Any?) {
        EditorWindow.openScratchTab(in: NSApp.keyWindow)
    }

    // The "+" button in the tab bar routes here through the responder chain.
    @objc func newWindowForTab(_ sender: Any?) {
        EditorWindow.openScratchTab(in: NSApp.keyWindow)
    }

    // Settings… (Cmd-,)
    @objc func showSettings(_ sender: Any?) {
        SettingsWindowController.shared.show()
    }

    // Edit > Spelling > Check Spelling While Typing.
    @objc func toggleSpelling(_ sender: Any?) {
        Settings.spellChecking.toggle()
    }

    // View > Increase/Decrease Font Size (Cmd-= / Cmd--). Settings clamps to the
    // allowed range and posts a change that resizes each window to keep its grid.
    @objc func increaseFontSize(_ sender: Any?) {
        Settings.fontSize = Settings.fontSize + 1
    }

    @objc func decreaseFontSize(_ sender: Any?) {
        Settings.fontSize = Settings.fontSize - 1
    }

    func validateMenuItem(_ item: NSMenuItem) -> Bool {
        if item.action == #selector(toggleSpelling(_:)) {
            item.state = Settings.spellChecking ? .on : .off
        }
        return true
    }

    // File > Save (Cmd-S): same as :w.
    @objc func saveDocument(_ sender: Any?) {
        EditorWindow.keyEditor()?.save()
    }

    // File > Save As… (Shift-Cmd-S): :f name then :w.
    @objc func saveDocumentAs(_ sender: Any?) {
        EditorWindow.keyEditor()?.saveAs()
    }

    // File > Open…: choose one or more files (placement follows Settings).
    @objc func openWindow(_ sender: Any?) {
        let panel = NSOpenPanel()
        panel.canChooseFiles = true
        panel.canChooseDirectories = false
        panel.allowsMultipleSelection = true
        panel.begin { response in
            guard response == .OK else { return }
            EditorWindow.openPaths(panel.urls.map { $0.path })
        }
    }
}

// Build the main menu. The app and Edit items use standard responder-chain
// selectors; File > New/Open target the app delegate.
func makeMenu(target: AppDelegate) -> NSMenu {
    let mainMenu = NSMenu()
    let name = ProcessInfo.processInfo.processName

    // Application menu.
    let appItem = NSMenuItem()
    mainMenu.addItem(appItem)
    let appMenu = NSMenu()
    appItem.submenu = appMenu
    appMenu.addItem(withTitle: "About \(name)", action: nil, keyEquivalent: "")
    appMenu.addItem(NSMenuItem.separator())
    let settingsItem = appMenu.addItem(withTitle: "Settings…",
                                       action: #selector(AppDelegate.showSettings(_:)), keyEquivalent: ",")
    settingsItem.target = target
    appMenu.addItem(NSMenuItem.separator())
    appMenu.addItem(withTitle: "Quit \(name)",
                    action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")

    // File menu.
    let fileItem = NSMenuItem()
    mainMenu.addItem(fileItem)
    let fileMenu = NSMenu(title: "File")
    fileItem.submenu = fileMenu
    let newItem = fileMenu.addItem(withTitle: "New", action: #selector(AppDelegate.newWindow(_:)), keyEquivalent: "n")
    newItem.target = target
    let tabItem = fileMenu.addItem(withTitle: "New Tab", action: #selector(AppDelegate.newTab(_:)), keyEquivalent: "t")
    tabItem.target = target
    let openItem = fileMenu.addItem(withTitle: "Open…", action: #selector(AppDelegate.openWindow(_:)), keyEquivalent: "o")
    openItem.target = target
    let saveItem = fileMenu.addItem(withTitle: "Save", action: #selector(AppDelegate.saveDocument(_:)), keyEquivalent: "s")
    saveItem.target = target
    let saveAsItem = fileMenu.addItem(withTitle: "Save As…", action: #selector(AppDelegate.saveDocumentAs(_:)), keyEquivalent: "S")
    saveAsItem.keyEquivalentModifierMask = [.command, .shift]
    saveAsItem.target = target
    fileMenu.addItem(NSMenuItem.separator())
    fileMenu.addItem(withTitle: "Close Window", action: #selector(NSWindow.performClose(_:)), keyEquivalent: "w")

    // Edit menu: Cut/Copy/Paste/Select All route through the responder chain to
    // GoviView's overrides.
    let editItem = NSMenuItem()
    mainMenu.addItem(editItem)
    let editMenu = NSMenu(title: "Edit")
    editItem.submenu = editMenu
    editMenu.addItem(withTitle: "Cut", action: #selector(NSText.cut(_:)), keyEquivalent: "x")
    editMenu.addItem(withTitle: "Copy", action: #selector(NSText.copy(_:)), keyEquivalent: "c")
    editMenu.addItem(withTitle: "Paste", action: #selector(NSText.paste(_:)), keyEquivalent: "v")
    editMenu.addItem(NSMenuItem.separator())
    editMenu.addItem(withTitle: "Select All", action: #selector(NSText.selectAll(_:)), keyEquivalent: "a")
    editMenu.addItem(NSMenuItem.separator())
    let spellingItem = NSMenuItem(title: "Spelling", action: nil, keyEquivalent: "")
    let spellingMenu = NSMenu(title: "Spelling")
    spellingItem.submenu = spellingMenu
    let checkItem = spellingMenu.addItem(withTitle: "Check Spelling While Typing",
                                         action: #selector(AppDelegate.toggleSpelling(_:)), keyEquivalent: "")
    checkItem.target = target
    editMenu.addItem(spellingItem)

    // View menu: font size. Cmd-= grows, Cmd-- shrinks (by 1 point); each open
    // window resizes to keep its rows x cols.
    let viewItem = NSMenuItem()
    mainMenu.addItem(viewItem)
    let viewMenu = NSMenu(title: "View")
    viewItem.submenu = viewMenu
    let biggerItem = viewMenu.addItem(withTitle: "Increase Font Size",
                                      action: #selector(AppDelegate.increaseFontSize(_:)), keyEquivalent: "=")
    biggerItem.target = target
    let smallerItem = viewMenu.addItem(withTitle: "Decrease Font Size",
                                       action: #selector(AppDelegate.decreaseFontSize(_:)), keyEquivalent: "-")
    smallerItem.target = target

    // Window menu. AppKit fills it with the window list and, because the windows
    // use native tabbing, the tab commands (Show Tab Bar, Show All Tabs, Merge
    // All Windows, Move Tab to New Window).
    let windowItem = NSMenuItem()
    mainMenu.addItem(windowItem)
    let windowMenu = NSMenu(title: "Window")
    windowItem.submenu = windowMenu
    windowMenu.addItem(withTitle: "Minimize", action: #selector(NSWindow.performMiniaturize(_:)), keyEquivalent: "m")
    windowMenu.addItem(withTitle: "Zoom", action: #selector(NSWindow.performZoom(_:)), keyEquivalent: "")
    NSApplication.shared.windowsMenu = windowMenu

    return mainMenu
}

// applyModeDefault pushes the Settings selection mode to the bridge so engines
// created next (new windows) start with it, before their LoadStartup.
func applyModeDefault() {
    Settings.selectionMode.rawValue.withCString {
        GoviSetDefaultMode(UnsafeMutablePointer(mutating: $0))
    }
}

let app = NSApplication.shared
app.setActivationPolicy(.regular)
let delegate = AppDelegate()
app.delegate = delegate
app.mainMenu = makeMenu(target: delegate)
applyModeDefault()
NotificationCenter.default.addObserver(
    forName: Settings.changed, object: nil, queue: .main) { _ in applyModeDefault() }
app.run()
