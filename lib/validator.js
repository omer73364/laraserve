const fs = require("fs");
const path = require("path");

function validateOptions(options) {
  const errors = [];

  // Validate domain
  if (
    !options.domain ||
    typeof options.domain !== "string" ||
    options.domain.trim() === ""
  ) {
    errors.push("Domain is required and must be a non-empty string");
  }

  // Validate path exists
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

  // Validate server type
  const validServers = ["apache", "nginx"];
  if (!options.server || !validServers.includes(options.server)) {
    errors.push(
      `Server must be either 'apache' or 'nginx', got: ${options.server}`
    );
  }

  if (errors.length > 0) {
    throw new Error("Validation failed:\n  - " + errors.join("\n  - "));
  }

  return true;
}

function checkRootPrivileges() {
  if (process.getuid && process.getuid() !== 0) {
    throw new Error(
      "This command requires sudo privileges. Please run with sudo."
    );
  }
}

module.exports = {
  validateOptions,
  checkRootPrivileges,
};
