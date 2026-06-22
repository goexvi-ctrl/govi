import Cocoa

// Govi: a native macOS application with the govi (Go nvi) editor engine embedded
// in-process. The engine is linked in as a C archive (libgovi); this app drives
// it and renders its screen in a custom NSView. nvi is *embedded*, not exec'd.
//
// The app is multi-window: each EditorWindow owns its own engine instance
// (a libgovi handle), so File > New and File > Open each spawn an independent
// editor.

// EditorWindow owns one window, its GoviView, and the embedded engine behind it.
// Instances keep themselves alive in a static set until their window closes.
final class EditorWindow: NSObject, NSWindowDelegate {
    private static var windows: Set<EditorWindow> = []
    private static var cascadePoint = NSPoint.zero

    let window: NSWindow
    let view: GoviView

    // open creates an editor for path (empty path = an empty buffer) and shows
    // its window. Returns nil if the file could not be opened.
    @discardableResult
    static func open(path: String) -> EditorWindow? {
        let handle = path.withCString { GoviStart(UnsafeMutablePointer(mutating: $0)) }
        guard handle != 0 else {
            let alert = NSAlert()
            alert.messageText = "Could not open “\(path)”."
            alert.runModal()
            return nil
        }
        let w = EditorWindow(handle: handle, path: path)
        windows.insert(w)
        w.show()
        return w
    }

    private init(handle: Int64, path: String) {
        let frame = NSRect(x: 0, y: 0, width: 800, height: 600)
        window = NSWindow(
            contentRect: frame,
            styleMask: [.titled, .closable, .miniaturizable, .resizable],
            backing: .buffered, defer: false)
        view = GoviView(frame: frame, handle: handle)
        super.init()
        window.title = path.isEmpty ? "Untitled" : (path as NSString).lastPathComponent
        window.contentView = view
        window.delegate = self
        window.isReleasedWhenClosed = false
    }

    private func show() {
        EditorWindow.cascadePoint = window.cascadeTopLeft(from: EditorWindow.cascadePoint)
        window.makeKeyAndOrderFront(nil)
        window.makeFirstResponder(view)
        view.updateGeometry()
    }

    func windowWillClose(_ notification: Notification) {
        GoviClose(view.handle)
        EditorWindow.windows.remove(self)
    }
}

final class AppDelegate: NSObject, NSApplicationDelegate {
    func applicationDidFinishLaunching(_ notification: Notification) {
        let args = CommandLine.arguments
        let path = args.count > 1 ? args[1] : ""
        EditorWindow.open(path: path)
        NSApp.activate(ignoringOtherApps: true)
    }

    func applicationShouldTerminateAfterLastWindowClosed(_ sender: NSApplication) -> Bool {
        true
    }

    // File > New: an empty window.
    @objc func newWindow(_ sender: Any?) {
        EditorWindow.open(path: "")
    }

    // File > Open…: choose one or more files, each in its own window.
    @objc func openWindow(_ sender: Any?) {
        let panel = NSOpenPanel()
        panel.canChooseFiles = true
        panel.canChooseDirectories = false
        panel.allowsMultipleSelection = true
        panel.begin { response in
            guard response == .OK else { return }
            for url in panel.urls {
                EditorWindow.open(path: url.path)
            }
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
    appMenu.addItem(withTitle: "Quit \(name)",
                    action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")

    // File menu.
    let fileItem = NSMenuItem()
    mainMenu.addItem(fileItem)
    let fileMenu = NSMenu(title: "File")
    fileItem.submenu = fileMenu
    let newItem = fileMenu.addItem(withTitle: "New", action: #selector(AppDelegate.newWindow(_:)), keyEquivalent: "n")
    newItem.target = target
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

    return mainMenu
}

let app = NSApplication.shared
app.setActivationPolicy(.regular)
let delegate = AppDelegate()
app.delegate = delegate
app.mainMenu = makeMenu(target: delegate)
app.run()
