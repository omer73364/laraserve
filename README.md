# Laraserve

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT) [![npm version](https://img.shields.io/npm/v/laraserve.svg?style=flat)](https://www.npmjs.com/package/laraserve) [![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen.svg)](http://makeapullrequest.com)

A CLI tool for Effortless Local Laravel Development with Automatic SSL for Linux.

## Features

- 🚀 **Interactive Mode** - Run without arguments and answer guided questions
- 🔧 **Command Line Mode** - Pass arguments directly for scripting
- 🖥️ **Multi-Server Support** - Works with both Apache and Nginx
- 🔒 **Automatic SSL** - Self-signed certificates trusted by your browser via mkcert
- 🔐 **Smart Permissions** - Sets proper ownership and permissions

---

## 🛠 Installation

> :warning: **Warning**: Only Debian based distributions (Ubuntu, Debian, Mint, etc.) is supported for now.

### Prerequisites

- Node.js 18 or higher
- sudo privileges
- Apache or Nginx installed

### Quick Install

```bash
sudo npm install -g laraserve
```

### Development Install

```bash
# Clone the repository
git clone https://github.com/omer73364/laraserve.git
cd laraserve

# Install dependencies
npm install

# Link for local development
sudo npm link
```

---

## 💻 Usage

### Interactive Mode

Just run:

```bash
sudo laraserve
```

You'll be guided through:

1. Domain name (e.g., `myapp.test`)
2. Document root path (e.g., `/var/www/myapp`)
3. Web server selection (Nginx/Apache)
4. SSL certificate setup

### Command Line Mode

```bash
sudo laraserve --domain <domain> --path <path> --server <apache|nginx> [--ssl]
```

### Options

| Flag       | Alias | Description                                   | Required |
| ---------- | ----- | --------------------------------------------- | -------- |
| `--domain` | `-d`  | Domain name (e.g., `example.test`)            | Yes      |
| `--path`   | `-p`  | Document root path (e.g., `/var/www/example`) | Yes      |
| `--server` | `-s`  | Server type: `apache` or `nginx`              | Yes      |
| `--ssl`    | -     | Enable SSL with mkcert                        | No       |
| `--help`   | `-h`  | Show help message                             | No       |

---

## 🚀 Examples

#### Apache without SSL

```bash
sudo laraserve --domain myapp.test --path /var/www/myapp --server apache
```

#### Nginx with SSL

```bash
sudo laraserve --domain myapp.test --path /var/www/myapp --server nginx --ssl
```

#### Using short flags

```bash
sudo laraserve -d myapp.test -p /var/www/myapp -s nginx --ssl
```

---

## 🔍 How It Works

Laraserve streamlines local development by automating:

1. **Server Configuration**

   - Creates optimized virtual host configs
   - Handles both HTTP and HTTPS
   - Sets proper document root and permissions

2. **SSL Setup**

   - Auto-installs mkcert if needed
   - Generates and trusts local certificates
   - Configures secure HTTPS by default

3. **System Integration**
   - Updates `/etc/hosts` automatically
   - Manages server modules and configurations
   - Handles service restarts

---

## 🤝 Contributing

Contributions are welcome! Please open issues or pull requests.

## 📄 License

This project is licensed under the MIT License.

## ❓ Support

Report issues at: https://github.com/omer73364/laraserve/issues
