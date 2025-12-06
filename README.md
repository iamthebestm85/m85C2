# m85 C2 - Telnet Stresser Control Panel

A lightweight Telnet-based C2 panel for launching attacks via integrated APIs. Built in Go for easy deployment. Supports animated ASCII (TFX), user auth, stats, and port scanning.

## Features
- Telnet login with per-user cooldowns.
- Commands: `?` (help), `clear`, `methods`, `lookup <host>`, `ongoing`, `stats`, `attack`, `gif`, `quit`.
- Attack syntax: `!<method> <host> <port> <time>` (e.g., `!httpflood example.com 80 60`).
- Animated welcome/attack screens via TFX files. ( 
- Logs to `logs.txt`; tracks attacks in memory.

## Quick Start
1. Run `./install.sh` (installs Go if needed, creates samples).
2. Edit `config.json` with your API details (e.g., stresser endpoints).
3. Run `go run c2.go`.
4. Connect: `telnet localhost 1111`.|
5. To convert GIF to TFX use gif.py  command -> python gif.py <input_gif_path> <output_tfx_path>
6. pip install numpy && pip install Pillow 

## Configuration
All customizations are file-based or in `config.json`. No rebuild required.

### config.json
- `port`: Telnet listen port (default: 1111).
- `banner_file`: Path to banner ASCII file (default: `branding/banner.txt`).
- `welcome_tfx`: Welcome animation file (default: `branding/welcome.tfx`).
- `attack_tfx`: Attack success animation (default: `branding/attack.tfx`).
- `login_file`: Users file (default: `login.txt`).
- `log_file`: Command logs (default: `logs.txt`).
- `apis`: Array of API configs:
  - `name`: Identifier (e.g., "stresser1" – used for JSON parsing logic).
  - `url`: Base attack URL (e.g., "https://api.stresser.com/launch").
  - `key`: API key.
  - `methods`: Attack methods (e.g., ["httpflood", "udp"] – prefixed with `!` in commands).
  - `method_param`: Query param for method (e.g., "method").

Example API addition:
```json
{
  "name": "old_api",
  "url": "https://api.example.com",
  "key": "abc123",
  "methods": ["httpbypass"],
  "method_param": "type"
}
