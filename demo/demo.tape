Output "/demo/demo.gif"

Set Shell "bash"
Set FontSize 14
Set Width 1400
Set Height 820
Set Padding 18
Set Theme "Catppuccin Mocha"
Set TypingSpeed 55ms

Hide
Type "export PATH=/demo:$PATH; export PS1='$ '; clear"
Enter
Show

Sleep 800ms
Type "sshmon --config /demo/config.yaml"
Sleep 500ms
Enter

# Fleet: grouped hosts, live metrics
Sleep 5s
Type "v"
Sleep 2500ms
Down
Sleep 800ms
Down
Sleep 800ms
Up
Sleep 1000ms

# Dashboard: CPU/RAM/disk/IO/net + ports + streaming logs
Enter
Sleep 6500ms

# Ports (read-only diagnostic screen)
Type "o"
Sleep 4000ms
Escape
Sleep 1200ms

# Live logs full screen
Ctrl+l
Sleep 4500ms
Escape
Sleep 1000ms

# Help overlay
Type "?"
Sleep 3500ms
Escape
Sleep 800ms

# back to Fleet and quit
Escape
Sleep 1500ms
Type "q"
Sleep 1200ms
