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

    // make creates an editor for path (empty path = an empty buffer) without
    // presenting it. Returns nil if the file could not be opened.
    private static func make(path: String) -> EditorWindow? {
        let fg = Settings.defaultForegroundColorSpec
        let bg = Settings.defaultBackgroundColorSpec
        let handle = path.withCString { pathPtr in
            fg.withCString { fgPtr in
                bg.withCString { bgPtr in
                    GoviStart(UnsafeMutablePointer(mutating: pathPtr),
                              UnsafeMutablePointer(mutating: fgPtr),
                              UnsafeMutablePointer(mutating: bgPtr))
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
    static func open(path: String) -> EditorWindow? {
        guard let w = make(path: path) else { return nil }
        w.showStandalone()
        return w
    }

    // existing returns the window already editing path, if any.
    private static func existing(path p: String) -> EditorWindow? {
        p.isEmpty ? nil : windows.first(where: { $0.path == p })
    }

    // openPaths opens one or more files according to Settings.openFilesIn. In tab
    // mode, files join the frontmost window's tab group (or start a new window
    // when none exists). In new-window mode, each file gets its own window.
    // Already-open files are focused in place rather than duplicated.
    static func openPaths(_ paths: [String]) {
        let normalized = paths.map { LaunchPath.normalize($0) }
        guard !normalized.isEmpty else { return }
        WaitCoordinator.shared.registerWait(paths: normalized)

        // With tabbing off, force separate windows regardless of the popup.
        let mode: Settings.OpenFilesIn = Settings.useTabs ? Settings.openFilesIn : .newWindow
        switch mode {
        case .newWindow:
            for p in normalized {
                if let w = existing(path: p) {
                    w.window.makeKeyAndOrderFront(nil)
                } else {
                    _ = open(path: p)
                }
            }
        case .tab:
            openAsTabs(normalized)
        }
        WaitCoordinator.shared.checkComplete()
    }

    // openAsTabs opens paths in the frontmost window's tab group. The first file
    // starts a new window when no anchor exists yet.
    private static func openAsTabs(_ paths: [String]) {
        var anchor = NSApp.keyWindow ?? windows.first?.window
        for p in paths {
            if let w = existing(path: p) {
                w.window.makeKeyAndOrderFront(nil)
                anchor = w.window
                continue
            }
            if let a = anchor {
                openTab(in: a, path: p)
            } else if let w = open(path: p) {
                anchor = w.window
            }
        }
    }

    // openTab presents path as a new tab in keyWindow's tab group, or as a
    // standalone window if there is no key window to tab into.
    static func openTab(in keyWindow: NSWindow?, path: String) {
        guard let w = make(path: path) else { return }
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
        view.documentTitle = path.isEmpty ? "Untitled" : (path as NSString).lastPathComponent
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

    // confirmClose returns true when the window/tab may close. When
    // Settings.warnOnUnsavedClose is set and the buffer is modified, the user
    // is prompted to save, discard, or cancel.
    private func confirmClose() -> Bool {
        if !Settings.warnOnUnsavedClose || GoviModified(view.handle) == 0 {
            return true
        }

        let displayName = path.isEmpty ? "Untitled" : (path as NSString).lastPathComponent
        let alert = NSAlert()
        alert.messageText = "Do you want to save the changes made to “\(displayName)”?"
        alert.informativeText = "Your changes will be lost if you don't save them."
        alert.alertStyle = .warning
        alert.addButton(withTitle: "Save")
        alert.addButton(withTitle: "Don't Save")
        alert.addButton(withTitle: "Cancel")

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

    private func saveForClose() -> Bool {
        var target = path
        if target.isEmpty {
            let panel = NSSavePanel()
            panel.canCreateDirectories = true
            panel.nameFieldStringValue = "Untitled"
            guard panel.runModal() == .OK, let url = panel.url else {
                GoviClearQuit(view.handle)
                return false
            }
            target = url.path
        }
        let saved = target.withCString { ptr in
            GoviSave(view.handle, UnsafeMutablePointer(mutating: ptr))
        } == 0
        if !saved {
            let alert = NSAlert()
            alert.messageText = "The document could not be saved."
            alert.informativeText = "“\(target)” could not be written."
            alert.alertStyle = .warning
            alert.runModal()
            GoviClearQuit(view.handle)
            return false
        }
        path = (target as NSString).standardizingPath
        view.documentTitle = (path as NSString).lastPathComponent
        view.updateTitle()
        return true
    }
}

// LaunchPath normalizes paths from the govi launcher, launch-files, and macOS
// open-documents events. Non-GUI parents often deliver
// basename-only Apple Events on a cold launch; launch-files carries the absolute
// paths the govi shell script resolved.
enum LaunchPath {
    private static var supportDir: URL {
        FileManager.default.urls(for: .applicationSupportDirectory, in: .userDomainMask)[0]
            .appendingPathComponent("GoVi", isDirectory: true)
    }

    static var launchFilesURL: URL {
        supportDir.appendingPathComponent("launch-files", isDirectory: false)
    }

    static var launchContextURL: URL {
        supportDir.appendingPathComponent("launch-context", isDirectory: false)
    }

    static var launchWaitURL: URL {
        supportDir.appendingPathComponent("launch-wait", isDirectory: false)
    }

    static func readLaunchWaitFifo() -> String? {
        guard let text = try? String(contentsOf: launchWaitURL, encoding: .utf8) else { return nil }
        for line in text.split(whereSeparator: \.isNewline) {
            if line.hasPrefix("fifo=") {
                let fifo = String(line.dropFirst(5)).trimmingCharacters(in: .whitespaces)
                if !fifo.isEmpty { return fifo }
            }
        }
        return nil
    }

    static func clearLaunchWait() {
        try? FileManager.default.removeItem(at: launchWaitURL)
    }

    static func readLaunchFiles() -> [String] {
        guard let text = try? String(contentsOf: launchFilesURL, encoding: .utf8) else { return [] }
        return text.split(whereSeparator: \.isNewline)
            .map { String($0).trimmingCharacters(in: .whitespaces) }
            .filter { !$0.isEmpty }
            .map { normalize($0) }
    }

    static func clearLaunchFiles() {
        try? FileManager.default.removeItem(at: launchFilesURL)
    }

    // isGoviTempFile reports whether path is a temp file `govi -g` (no files)
    // created for an empty editor (nvi-style vi.XXXXXX in the temp dir). Such a
    // file is deleted when its window/tab closes.
    static func isGoviTempFile(_ path: String) -> Bool {
        guard (path as NSString).lastPathComponent.hasPrefix("vi.") else { return false }
        let parent = URL(fileURLWithPath: (path as NSString).deletingLastPathComponent).standardizedFileURL.path
        return parent == URL(fileURLWithPath: NSTemporaryDirectory()).standardizedFileURL.path
    }

    static func readLaunchCwd() -> String? {
        guard let text = try? String(contentsOf: launchContextURL, encoding: .utf8) else { return nil }
        for line in text.split(whereSeparator: \.isNewline) {
            if line.hasPrefix("cwd=") {
                let cwd = String(line.dropFirst(4)).trimmingCharacters(in: .whitespaces)
                if !cwd.isEmpty { return (cwd as NSString).standardizingPath }
            }
        }
        return nil
    }

    static func normalize(_ path: String) -> String {
        let p = (path as NSString).standardizingPath
        if (p as NSString).isAbsolutePath { return p }
        if let cwd = readLaunchCwd(), !cwd.isEmpty {
            return (cwd as NSString).appendingPathComponent(p)
        }
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
    func registerWait(paths: [String]) {
        lock.lock()
        defer { lock.unlock() }
        guard let fifo = LaunchPath.readLaunchWaitFifo() else { return }
        LaunchPath.clearLaunchWait()
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
    // Queued open-documents paths during a cold launch (handled in finishColdLaunch).
    private var pendingOpenPaths: [String] = []
    private var coldLaunchComplete = false

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
        // launch-files (from govi -g) wins over Apple Events: cold launches from
        // background helpers often deliver basename-only paths and would otherwise
        // erase launch-files before we read it.
        let fromLauncher = LaunchPath.readLaunchFiles()
        if !fromLauncher.isEmpty {
            EditorWindow.openPaths(fromLauncher)
        } else if !pendingOpenPaths.isEmpty {
            EditorWindow.openPaths(pendingOpenPaths)
        }
        pendingOpenPaths.removeAll()
        LaunchPath.clearLaunchFiles()
        coldLaunchComplete = true
        if !EditorWindow.anyOpen {
            EditorWindow.open(path: "")
        }
    }

    // applicationShouldHandleReopen fires when `open`-ing an already-running app
    // (and on Dock clicks). `govi -g` with no files leaves a sentinel so it opens
    // a fresh empty editor even when windows are already open; a plain reopen with
    // no windows opens one too.
    func applicationShouldHandleReopen(_ sender: NSApplication, hasVisibleWindows: Bool) -> Bool {
        if !hasVisibleWindows {
            EditorWindow.open(path: "")
        }
        return true
    }

    // application(_:open:) is the open-documents Apple Event. macOS routes
    // `open -a GoVi.app file ...` (and Finder double-clicks / drags) here --
    // delivering to the *running* instance when one exists, which is what makes
    // the command-line `govi` tool reuse a running app.
    func application(_ application: NSApplication, open urls: [URL]) {
        let paths = urls.filter { $0.isFileURL }.map { LaunchPath.pathFromOpenURL($0) }
        if coldLaunchComplete {
            LaunchPath.clearLaunchFiles()
            EditorWindow.openPaths(paths)
        } else {
            pendingOpenPaths.append(contentsOf: paths)
        }
        NSApp.activate(ignoringOtherApps: true)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }

    // File > New: an empty window.
    @objc func newWindow(_ sender: Any?) {
        EditorWindow.open(path: "")
    }

    // File > New Tab: an empty tab in the current window's tab group.
    @objc func newTab(_ sender: Any?) {
        EditorWindow.openTab(in: NSApp.keyWindow, path: "")
    }

    // The "+" button in the tab bar routes here through the responder chain.
    @objc func newWindowForTab(_ sender: Any?) {
        EditorWindow.openTab(in: NSApp.keyWindow, path: "")
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

let app = NSApplication.shared
app.setActivationPolicy(.regular)
let delegate = AppDelegate()
app.delegate = delegate
app.mainMenu = makeMenu(target: delegate)
app.run()
