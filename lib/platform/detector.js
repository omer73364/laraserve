const { execSync } = require("child_process");
const os = require("os");

const Platform = {
  DARWIN: "darwin",
  LINUX: "linux",
  WINDOWS: "windows",
};

const Arch = {
  X64: "x64",
  ARM64: "arm64",
  ARM: "arm",
};

const InitSystem = {
  SYSTEMD: "systemd",
  LAUNCHD: "launchd",
  WINDOWS_SERVICE: "windows_service",
  NONE: "none",
};

function detectArchitecture() {
  const arch = os.arch();
  switch (arch) {
    case "x64":
    case "amd64":
      return Arch.X64;
    case "arm64":
    case "aarch64":
      return Arch.ARM64;
    case "arm":
      return Arch.ARM;
    default:
      return arch;
  }
}

function detectPlatform() {
  const platform = os.platform();
  switch (platform) {
    case "darwin":
      return Platform.DARWIN;
    case "linux":
      return Platform.LINUX;
    case "win32":
      return Platform.WINDOWS;
    default:
      return platform;
  }
}

function isWSL() {
  const platform = detectPlatform();
  if (platform !== Platform.LINUX) {
    return false;
  }

  try {
    const release = execSync("uname -r", { encoding: "utf8" }).trim();
    return release.toLowerCase().includes("wsl");
  } catch {
    return false;
  }
}

function isRunningUnderWSLOnWindows() {
  if (process.env.WSL_INTEROP) {
    return true;
  }
  if (process.env.WSL_DISTRO_NAME) {
    return true;
  }
  return isWSL();
}

function detectInitSystem() {
  const platform = detectPlatform();

  if (platform === Platform.DARWIN) {
    return InitSystem.LAUNCHD;
  }

  if (platform === Platform.WINDOWS) {
    return InitSystem.WINDOWS_SERVICE;
  }

  if (platform === Platform.LINUX) {
    if (isWSL()) {
      return InitSystem.NONE;
    }

    try {
      execSync("systemctl --version", { stdio: "ignore" });
      return InitSystem.SYSTEMD;
    } catch {
      try {
        execSync("service --version", { stdio: "ignore" });
        return InitSystem.SYSVINIT;
      } catch {
        return InitSystem.NONE;
      }
    }
  }

  return InitSystem.NONE;
}

function isAdmin() {
  const platform = detectPlatform();

  if (platform === Platform.WINDOWS) {
    try {
      execSync("net session", { stdio: "ignore" });
      return true;
    } catch {
      return false;
    }
  }

  return process.getuid && process.getuid() === 0;
}

function needsElevation() {
  return !isAdmin();
}

function getElevationCommand() {
  const platform = detectPlatform();
  const currentCommand = process.argv.slice(1).join(" ");

  if (platform === Platform.WINDOWS) {
    return `Run as Administrator: Right-click PowerShell/CMD and select "Run as Administrator", then run: ${currentCommand}`;
  }

  return `sudo ${currentCommand}`;
}

function detectDistribution() {
  const platform = detectPlatform();
  if (platform !== Platform.LINUX) {
    return null;
  }

  const files = [
    "/etc/os-release",
    "/etc/lsb-release",
    "/etc/alpine-release",
    "/etc/arch-release",
    "/etc/fedora-release",
    "/etc/redhat-release",
  ];

  for (const file of files) {
    try {
      const content = require("fs").readFileSync(file, "utf8");
      if (content.includes("ID=")) {
        const match = content.match(/ID=["']?([^"'\n]+)["']?/);
        if (match) {
          return match[1].toLowerCase().replace(/"/g, "");
        }
      }
      if (content.includes("Alpine")) return "alpine";
      if (content.includes("Arch")) return "arch";
      if (content.includes("Fedora")) return "fedora";
      if (content.includes("Red Hat") || content.includes("RedHat")) return "rhel";
    } catch {
      continue;
    }
  }

  return "unknown";
}

function getLinuxPackageManager() {
  const dist = detectDistribution();
  const isWSLEnv = isWSL();

  if (["ubuntu", "debian", "mint", "pop"].includes(dist) || isWSLEnv) {
    return { type: "apt", command: "apt" };
  }
  if (["fedora", "rhel", "centos", "rocky", "almalinux"].includes(dist)) {
    return { type: "dnf", command: "dnf" };
  }
  if (dist === "arch") {
    return { type: "pacman", command: "pacman" };
  }
  if (dist === "alpine") {
    return { type: "apk", command: "apk" };
  }

  return { type: "apt", command: "apt" };
}

function getWebServerUser(server) {
  const platform = detectPlatform();

  if (platform === Platform.DARWIN) {
    return "_www";
  }

  if (platform === Platform.WINDOWS) {
    return "Administrators";
  }

  if (server === "apache") {
    try {
      execSync("id -u www-data", { stdio: "ignore" });
      return "www-data";
    } catch {
      return "apache";
    }
  } else if (server === "nginx") {
    try {
      execSync("id -u www-data", { stdio: "ignore" });
      return "www-data";
    } catch {
      return "nginx";
    }
  }

  return "www-data";
}

module.exports = {
  Platform,
  Arch,
  InitSystem,
  detectArchitecture,
  detectPlatform,
  isWSL,
  isRunningUnderWSLOnWindows,
  detectInitSystem,
  isAdmin,
  needsElevation,
  getElevationCommand,
  detectDistribution,
  getLinuxPackageManager,
  getWebServerUser,
};
