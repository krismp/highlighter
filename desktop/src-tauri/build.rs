fn main() {
    // The app's own commands, so capabilities can grant them to the UI.
    let app = tauri_build::AppManifest::new().commands(&["check_update", "install_update"]);
    tauri_build::try_build(tauri_build::Attributes::new().app_manifest(app)).expect("failed to run tauri-build");
}
