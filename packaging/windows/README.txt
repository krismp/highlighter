HIGHLIGHTER FOR WINDOWS
=======================

1. Right-click the downloaded .zip file and choose "Extract All...",
   then click Extract. (Running it from inside the zip does not work.)

2. Move the extracted "Highlighter" folder wherever you like, for example
   Documents or C:\Program Files. Keep everything in it together.

3. Double-click Highlighter.exe.

   The first time, Windows may show "Windows protected your PC" because
   the app is not signed. Click "More info", then "Run anyway".
   If Windows Firewall asks, you can click Cancel: highlighter only talks
   to your own computer (127.0.0.1).

4. Highlighter opens in your web browser at http://127.0.0.1:7777.
   A black console window also opens: keep it open while you use the app.
   To stop, click "Quit" in the top-right corner of the page, or close
   the console window.

Tip: right-click Highlighter.exe > Show more options > Send to > Desktop
(create shortcut) to start it from your desktop.

Everything highlighter needs (yt-dlp, ffmpeg, deno) is in the "tools"
folder. Nothing else to install.

Clips are saved to Videos\Highlights (changeable in Settings).

To uninstall, delete the Highlighter folder, and optionally the
.config\highlighter folder in your user folder (saved sessions).
