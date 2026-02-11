const fs = require("fs");
const path = require("path");
const { detectPlatform, Platform } = require("./platform");
const { getFastcgiPass, getPhpInfo } = require("./platform");
const { triggerError } = require("./utils");

function loadTemplate(templateName, platform = null) {
  let templatePath;

  if (platform) {
    const platformDir = platform === Platform.DARWIN ? "darwin" : "windows";
    templatePath = path.join(__dirname, "..", "templates", platformDir, templateName);
    if (fs.existsSync(templatePath)) {
      return fs.readFileSync(templatePath, "utf8");
    }
  }

  templatePath = path.join(__dirname, "..", "templates", templateName);
  try {
    return fs.readFileSync(templatePath, "utf8");
  } catch (error) {
    triggerError(`Failed to load template ${templateName}: ${error.message}`);
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

function getHomebrewPrefix() {
  try {
    return require("child_process").execSync("brew --prefix", { encoding: "utf8" }).trim();
  } catch {
    return "/usr/local";
  }
}

function getConfigPaths(options) {
  const { domain, server, ssl } = options;
  const platform = detectPlatform();

  // Validate server type early
  if (server !== "apache" && server !== "nginx") {
    triggerError(`Unsupported server: ${server}`);
  }

  const template = ssl ? `${server}-ssl.conf` : `${server}.conf`;
  
  // Configuration factory for each server type
  const configBuilders = {
    apache: {
      [Platform.DARWIN]: () => {
        const brewPrefix = getHomebrewPrefix();
        return {
          configPath: path.join(brewPrefix, "etc", "httpd", "users", `${domain}.conf`),
          serverRoot: path.join(brewPrefix, "var", "httpd"),
        };
      },
      [Platform.WINDOWS]: () => {
        const serverRoot = process.env.SERVERROOT || "C:\\Apache24";
        return {
          configPath: path.join(serverRoot, "conf", "sites", `${domain}.conf`),
          serverRoot,
        };
      },
      default: () => ({
        configPath: `/etc/apache2/sites-available/${domain}.conf`,
        serverRoot: "/var/log/apache2",
      }),
    },
    nginx: {
      [Platform.DARWIN]: () => {
        const brewPrefix = getHomebrewPrefix();
        return {
          configPath: path.join(brewPrefix, "etc", "nginx", "servers", domain),
          serverRoot: path.join(brewPrefix, "var", "nginx"),
        };
      },
      [Platform.WINDOWS]: () => {
        const serverRoot = process.env.NGINX_PATH || "C:\\nginx";
        return {
          configPath: path.join(serverRoot, "conf", "sites", `${domain}.conf`),
          serverRoot,
        };
      },
      default: () => ({
        configPath: `/etc/nginx/sites-available/${domain}`,
        serverRoot: "/var/log/nginx",
      }),
    },
  };

  // Get the appropriate config builder
  const serverConfigs = configBuilders[server];
  const buildConfig = serverConfigs[platform] || serverConfigs.default;
  const config = buildConfig();

  return {
    template,
    platformTemplate: template,
    ...config,
  };
}

function normalizePath(filePath) {
  if (process.platform === "win32") {
    return filePath.replace(/\//g, "\\");
  }
  return filePath.replace(/\\/g, "/");
}

function validateOptions(options) {
  const required = ['domain', 'path', 'server'];
  const missing = required.filter(field => !options[field]);
  
  if (missing.length > 0) {
    triggerError(`Missing required options: ${missing.join(', ')}`);
  }
  
  if (!['nginx', 'apache'].includes(options.server)) {
    triggerError(`Unsupported server type: ${options.server}`);
  }
}

function buildSslConfig(sslEnabled, sslFiles) {
  if (!sslEnabled || !sslFiles) {
    return { certFile: '', keyFile: '' };
  }
  
  const { certFile, keyFile } = sslFiles;
  
  // Validate SSL files exist
  if (!fs.existsSync(certFile)) {
    triggerError(`SSL certificate file not found: ${certFile}`);
  }
  if (!fs.existsSync(keyFile)) {
    triggerError(`SSL key file not found: ${keyFile}`);
  }
  
  return {
    certFile: normalizePath(certFile),
    keyFile: normalizePath(keyFile),
  };
}

function buildPhpConfig(server) {
  if (server !== 'nginx') {
    return { fastcgiPass: null, phpInfo: null };
  }
  
  const phpInfo = getPhpInfo();
  const fastcgiPass = getFastcgiPass();
  
  console.log(`  PHP version: ${phpInfo.version || 'not detected'}`);
  console.log(`  PHP handler: ${phpInfo.isTcp ? `TCP ${phpInfo.port}` : `Socket ${phpInfo.socket}`}`);
  
  return { fastcgiPass, phpInfo };
}

function buildPaths(docRoot) {
  const resolvedPath = normalizePath(path.resolve(docRoot));
  const publicPath = normalizePath(path.join(resolvedPath, 'public'));
  
  // Validate document root exists
  if (!fs.existsSync(resolvedPath)) {
    triggerError(`Document root does not exist: ${resolvedPath}`);
  }
  
  return { resolvedPath, publicPath };
}

function buildConfigContent(template, platform, params) {
  const { domain, paths, sslConfig, phpConfig, serverRoot } = params;
  
  const templateContent = loadTemplate(template, platform);

  return replaceVariables(templateContent, {
    DOMAIN: domain,
    PATH: paths.resolvedPath,
    PUBLIC_PATH: paths.publicPath,
    SSL_CERT: sslConfig.certFile,
    SSL_KEY: sslConfig.keyFile,
    SERVER_NAME: domain,
    SERVER_ALIAS: `www.${domain}`,
    SERVERROOT: normalizePath(serverRoot),
    FASTCGI_PASS: phpConfig.fastcgiPass || '',
  });
}

function writeConfigFile(configPath, content) {
  const configDir = path.dirname(configPath);
  
  // Create directory if it doesn't exist
  if (!fs.existsSync(configDir)) {
    fs.mkdirSync(configDir, { recursive: true });
  }
  
  // Check write permissions
  try {
    fs.accessSync(configDir, fs.constants.W_OK);
  } catch (error) {
    triggerError(`No write permission for directory: ${configDir}`);
  }
  
  // Write the configuration file
  fs.writeFileSync(configPath, content, 'utf8');
}

function generateConfig(options, sslFiles = null) {
  console.log("\n📄 Generating site configuration...");
  
  try {
    // Validate required options
    validateOptions(options);
    
    const platform = detectPlatform();
    const { domain, path: docRoot, server, ssl } = options;
    const { template, configPath, serverRoot } = getConfigPaths(options);
    
    // Handle SSL configuration
    const sslConfig = buildSslConfig(ssl, sslFiles);
    
    // Handle PHP/FastCGI configuration for nginx
    const phpConfig = buildPhpConfig(server);
    
    // Build file paths
    const paths = buildPaths(docRoot);
    
    // Generate configuration content
    const config = buildConfigContent(template, platform, {
      domain,
      paths,
      sslConfig,
      phpConfig,
      serverRoot,
    });
    
    // Write configuration file
    writeConfigFile(configPath, config);
    
    console.log(`✓ Configuration written to ${configPath}`);
    return configPath;
    
  } catch (error) {
    triggerError(`Failed to generate configuration: ${error.message}`);
  }
}

module.exports = {
  generateConfig,
};
