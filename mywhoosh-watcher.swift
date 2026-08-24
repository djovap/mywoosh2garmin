import AppKit
import Foundation

private let myWhooshBundleID = "com.whoosh.whooshgame"
private let bikeControlApp = "BikeControl"
private let bikeControlBundleID = "de.jonasbark.swiftcontrol.darwin"
private let dockPreferencesDomain = "com.apple.dock"

private func log(_ message: String, to handle: FileHandle = .standardOutput) {
    handle.write(Data("\(message)\n".utf8))
}

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
            log("MyWhoosh is already running; opening \(bikeControlApp).")
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

        log("MyWhoosh opened (PID \(app.processIdentifier)); opening \(bikeControlApp).")
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

        log("MyWhoosh closed; closing \(bikeControlApp), removing it from the Dock, and starting the sync hook.")
        closeBikeControl()
        // Wait for macOS to record BikeControl as a recent app after it exits,
        // then remove that entry before the Dock reloads.
        DispatchQueue.main.asyncAfter(deadline: .now() + 1) { [weak self] in
            guard let self else { return }
            self.removeBikeControlFromDock()
            self.runExitHook()
        }
    }

    private func openBikeControl() {
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/bin/open")
        process.arguments = ["-a", bikeControlApp]

        do {
            try process.run()
        } catch {
            log("Could not open \(bikeControlApp): \(error)", to: .standardError)
        }
    }

    private func closeBikeControl() {
        for app in workspace.runningApplications where app.bundleIdentifier == bikeControlBundleID {
            _ = app.terminate()
        }
    }

    private func removeBikeControlFromDock() {
        let defaults = UserDefaults.standard
        guard var dockPreferences = defaults.persistentDomain(forName: dockPreferencesDomain) else {
            log("Could not read Dock preferences; BikeControl remains in the Dock.", to: .standardError)
            return
        }

        var changed = false
        for section in ["persistent-apps", "recent-apps"] {
            guard var apps = dockPreferences[section] as? [[String: Any]] else {
                continue
            }

            let originalCount = apps.count
            apps.removeAll { item in
                guard let tileData = item["tile-data"] as? [String: Any] else {
                    return false
                }
                if tileData["bundle-identifier"] as? String == bikeControlBundleID {
                    return true
                }
                let fileData = tileData["file-data"] as? [String: Any]
                let url = fileData?["_CFURLString"] as? String
                return url == "file:///Applications/BikeControl.app/"
            }
            if apps.count != originalCount {
                dockPreferences[section] = apps
                changed = true
            }
        }

        guard changed else {
            return
        }

        defaults.setPersistentDomain(dockPreferences, forName: dockPreferencesDomain)
        defaults.synchronize()

        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/usr/bin/killall")
        process.arguments = ["Dock"]
        do {
            try process.run()
        } catch {
            log("BikeControl was removed from Dock preferences, but the Dock could not reload: \(error)", to: .standardError)
        }
    }

    private func runExitHook() {
        guard FileManager.default.isExecutableFile(atPath: exitHook.path) else {
            log("Sync hook is missing or not executable: \(exitHook.path)", to: .standardError)
            return
        }

        let process = Process()
        process.executableURL = exitHook
        process.standardOutput = FileHandle.standardOutput
        process.standardError = FileHandle.standardError

        do {
            try process.run()
        } catch {
            log("Could not start sync hook: \(error)", to: .standardError)
        }
    }
}

let executableURL = URL(fileURLWithPath: CommandLine.arguments[0]).resolvingSymlinksInPath()
let projectDir = executableURL.deletingLastPathComponent()
let exitHook = projectDir.appendingPathComponent("sync-when-mywhoosh-closes.zsh")

let observer = MyWhooshLifecycleObserver(exitHook: exitHook)
log("Watching MyWhoosh lifecycle events.")
withExtendedLifetime(observer) {
    RunLoop.main.run()
}
