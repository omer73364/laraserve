const fs = require("fs");
const path = require("path");

function loadTemplate(templateName) {
  const templatePath = path.join(__dirname, "..", "templates", templateName);
  try {
    return fs.readFileSync(templatePath, "utf8");
  } catch (error) {
    throw new Error(
      `Failed to load template ${templateName}: ${error.message}`
    );
  }
}

function replaceVariables(template, variables) {
  let result = template;
  for (const [key, value] of Object.entries(variables)) {
    const placeholder = `{{${key}}}`;
    result = result.split(placeholder).join(value);
  }
  return result;
}

function generateConfig(options) {
  console.log("\n📄 Generating site configuration...");

  const { domain, path: docRoot, server, ssl } = options;

  // Select template based on server and SSL
  let templateName;
  let configPath;

  if (server === "apache") {
    templateName = ssl ? "apache-ssl.conf" : "apache.conf";
    configPath = `/etc/apache2/sites-available/${domain}.conf`;
  } else if (server === "nginx") {
    templateName = ssl ? "nginx-ssl.conf" : "nginx.conf";
    configPath = `/etc/nginx/sites-available/${domain}`;
  } else {
    throw new Error(`Unsupported server: ${server}`);
  }

  // Load and process template
  const template = loadTemplate(templateName);
  const config = replaceVariables(template, {
    DOMAIN: domain,
    PATH: path.resolve(docRoot),
  });

  // Write config file
  try {
    // Ensure directory exists
    const configDir = path.dirname(configPath);
    if (!fs.existsSync(configDir)) {
      fs.mkdirSync(configDir, { recursive: true });
    }

    fs.writeFileSync(configPath, config, "utf8");
    console.log(`   ✓ Configuration written to ${configPath}`);
  } catch (error) {
    throw new Error(`Failed to write config file: ${error.message}`);
  }

  return configPath;
}

module.exports = {
  generateConfig,
};
