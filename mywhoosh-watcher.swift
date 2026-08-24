import AppKit
import Foundation

private let myWhooshBundleID = "com.whoosh.whooshgame"
private let bikeControlApp = "BikeControl"

final class MyWhooshLifecycleObserver {
    private let workspace = NSWorkspace.shared
    private let exitHook: URL
    private var myWhooshPIDs = Set<pid_t>()

    init(exitHook: URL) {
        self.exitHook = exitHook

        for app in workspace.runningApplications where app.bundleIdentifier == myWhooshBundleID {
            myWhooshPIDs.insert(app.processIdentifier)
        }

        if !myWhooshPIDs.isEmpty {
            print("MyWhoosh is already running; opening \(bikeControlApp).")
            openBikeControl()
        }

        workspace.notificationCenter.addObserver(
            self,
            selector: #selector(applicationLaunched(_:)),
            name: NSWorkspace.didLaunchApplicationNotification,
            object: workspace
        )
        workspace.notificationCenter.addObserver(
            self,
            selector: #selector(applicationTerminated(_:)),
            name: NSWorkspace.didTerminateApplicationNotification,
            object: workspace
        )
    }

    @objc private func applicationLaunched(_ notification: Notification) {
        guard let app = notification.userInfo?[NSWorkspace.applicationUserInfoKey] as? NSRunningApplication,
              app.bundleIdentifier == myWhooshBundleID else {
            return
        }

        guard myWhooshPIDs.insert(app.processIdentifier).inserted else {
            return
        }

        print("MyWhoosh opened (PID \(app.processIdentifier)); opening \(bikeControlApp).")
        openBikeControl()
    }

    @objc private func applicationTerminated(_ notification: Notification) {
        guard let app = notification.userInfo?[NSWorkspace.applicationUserInfoKey] as? NSRunningApplication,
              app.bundleIdentifier == myWhooshBundleID else {
            return
        }

        myWhooshPIDs.remove(app.processIdentifier)
        guard myWhooshPIDs.isEmpty else {
            return
        }

        print("MyWhoosh closed; starting the sync hook.")
        runExitHook()
    }

    private func openBikeControl() {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/bin/open")
        process.arguments = ["-a", bikeControlApp]

        do {
            try process.run()
        } catch {
            fputs("Could not open \(bikeControlApp): \(error)\n", stderr)
        }
    }

    private func runExitHook() {
        guard FileManager.default.isExecutableFile(atPath: exitHook.path) else {
            fputs("Sync hook is missing or not executable: \(exitHook.path)\n", stderr)
            return
        }

        let process = Process()
        process.executableURL = exitHook
        process.standardOutput = FileHandle.standardOutput
        process.standardError = FileHandle.standardError

        do {
            try process.run()
        } catch {
            fputs("Could not start sync hook: \(error)\n", stderr)
        }
    }
}

let executableURL = URL(fileURLWithPath: CommandLine.arguments[0]).resolvingSymlinksInPath()
let projectDir = executableURL.deletingLastPathComponent()
let exitHook = projectDir.appendingPathComponent("sync-when-mywhoosh-closes.zsh")

_ = MyWhooshLifecycleObserver(exitHook: exitHook)
print("Watching MyWhoosh lifecycle events.")
RunLoop.main.run()
