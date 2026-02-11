const fs = require("fs");
const path = require("path");
const { detectPlatform, Platform, isWSL, needsElevation, getElevationCommand } = require("./platform");
const { triggerError } = require("./utils");

function validateOptions(options) {
  const errors = [];

  if (!options.domain || typeof options.domain !== "string" || options.domain.trim() === "") {
    errors.push("Domain is required and must be a non-empty string");
  }

  if (!options.path || typeof options.path !== "string") {
    errors.push("Path is required and must be a string");
  } else {
    const absolutePath = path.resolve(options.path);
    if (!fs.existsSync(absolutePath)) {
      errors.push(`Path does not exist: ${absolutePath}`);
    } else if (!fs.statSync(absolutePath).isDirectory()) {
      errors.push(`Path is not a directory: ${absolutePath}`);
    }
  }

  const validServers = ["apache", "nginx"];
  if (!options.server || !validServers.includes(options.server)) {
    errors.push(`Server must be either 'apache' or 'nginx', got: ${options.server}`);
  }

  if (errors.length > 0) {
    triggerError("Validation failed:\n  - " + errors.join("\n  - "));
    
  }

  return true;
}

function checkRootPrivileges() {
  const platform = detectPlatform();

  if (platform === Platform.WINDOWS) {
    if (needsElevation()) {
      triggerError(
        "\n🚨  Administrator privileges required! Please run as Administrator:\n" +
          getElevationCommand()
      );
      
    }
    return true;
  }

  if (process.getuid && process.getuid() !== 0) {
    triggerError(
      "\n🚨  Root privileges required! Please run with:\n" +
        getElevationCommand()
    );
    
  }

  return true;
}

function checkElevation() {
  const platform = detectPlatform();

  if (platform === Platform.WINDOWS && isWSL()) {
    console.log("\n⚠️  Running under WSL - make sure Windows Defender Firewall allows connections");
  }

  if (needsElevation()) {
    console.log(`\n⚠️  Elevated privileges required: ${getElevationCommand()}`);
  }
}

function checkPlatformSupport(options) {
  const platform = detectPlatform();

  const unsupportedFeatures = [];

  if (platform === Platform.WINDOWS) {
    if (options.server === "apache") {
      const apachePath = "C:\\Apache24\\bin\\httpd.exe";
      if (!fs.existsSync(apachePath)) {
        unsupportedFeatures.push("Apache on Windows requires manual installation from Apache Lounge");
      }
    }
  }

  if (unsupportedFeatures.length > 0) {
    console.log("\n⚠️  Platform warnings:");
    unsupportedFeatures.forEach((feature) => console.log(`   - ${feature}`));
  }

  return true;
}

module.exports = {
  validateOptions,
  checkRootPrivileges,
  checkElevation,
  checkPlatformSupport,
};
