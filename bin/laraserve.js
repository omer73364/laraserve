#!/usr/bin/env node

const minimist = require("minimist");
const { setupSite } = require("../lib/setup");
const { listSites, removeSite } = require("../lib/sites");
const { getPhpInfo, autoDetectServer } = require("../lib/platform");
const { triggerError } = require("../lib/utils");

function showHelp() {
  console.log(`
laraserve - Local site configuration generator

Usage:
  laraserve                    Run in interactive mode
  laraserve add <domain> <path> [--no-ssl]   Add a new site
  laraserve list                                       List all sites
  laraserve remove <domain>                            Remove a site
  laraserve php                                        Show PHP information

Options:
  --domain <domain>    Domain name (e.g., example.test)
  --path <path>        Document root path (e.g., /var/www/example)
  --no-ssl            Disable SSL (SSL is enabled by default)
  --help               Show this help message

Examples:
  laraserve add myapp.test /var/www/myapp
  laraserve add myapp.test /var/www/myapp --no-ssl
  laraserve list
  laraserve remove myapp.test
  laraserve php
`);
  process.exit(0);
}

function showPhpInfo() {
  const phpInfo = getPhpInfo();

  console.log("\n🐘 PHP Information\n");

  if (phpInfo.installed) {
    console.log(`  Version:    ${phpInfo.version}`);
    console.log(
      `  Handler:    ${phpInfo.isTcp ? `TCP port ${phpInfo.port}` : `Socket`}"`
    );
    if (!phpInfo.isTcp) {
      console.log(`  Socket:     ${phpInfo.socket}`);
    }
    console.log(`  Installed:  Yes`);
  } else {
    console.log(`  PHP is not installed or not in PATH`);
  }

  if (phpInfo.versions.length > 0) {
    console.log(`\n  Available PHP versions:`);
    phpInfo.versions.forEach((v) => console.log(`    - PHP ${v}`));
  }

  console.log("\n");
}

async function promptForOptions() {
  const { prompt } = (await import("inquirer")).default;

  const questions = [
    {
      type: "input",
      name: "domain",
      message: "Enter domain name:",
      validate: (input) => {
        if (!input || input.trim() === "") {
          return "Domain name is required";
        }
        if (
          !/^[a-zA-Z0-9][a-zA-Z0-9-]*(\.[a-zA-Z0-9][a-zA-Z0-9-]*)+$/.test(input)
        ) {
          return "Please enter a valid domain name";
        }
        return true;
      },
    },
    {
      type: "input",
      name: "path",
      message: "Enter document root path:",
      validate: (input) => {
        if (!input || input.trim() === "") {
          return "Document root path is required";
        }
        return true;
      },
    },
    {
      type: "confirm",
      name: "ssl",
      message: "Enable SSL with mkcert?",
      default: true,
    },
  ];

  const answers = await prompt(questions);
  return {
    domain: answers.domain.trim(),
    path: answers.path.trim(),
    server: autoDetectServer(),
    ssl: answers.ssl,
  };
}

async function main() {
  const args = minimist(process.argv.slice(2), {
    string: ["domain", "path"],
    boolean: ["ssl", "help", "no-ssl"],
    alias: {
      h: "help",
      d: "domain",
      p: "path",
    },
  });

  if (args.help) {
    showHelp();
  }

  const command = args._[0];

  if (command === "list" || args.list) {
    listSites();
    return;
  }

  if (command === "php" || args.php) {
    showPhpInfo();
    return;
  }

  if (command === "remove" || args.remove || args.rm) {
    const domain = args.domain || args.remove || args.rm || args._[1];
    if (!domain) {
      triggerError(
        "\n❌ Error: Domain name is required for remove command\nUsage: laraserve remove <domain>"
      );
    }
    removeSite(domain);
    return process.exit(0);
  }

  if (command === "add" || !args?.length) {
    let options;

    if (!args.domain || !args.path) {
      console.log("\n🚀 laraserve - Interactive Site Configuration\n");
      console.log("Missing required arguments. Starting interactive mode...\n");
      options = await promptForOptions();
    } else {
      let ssl = true;
      if (args["no-ssl"] || args.ssl === false) {
        ssl = false;
      }
      options = {
        domain: args.domain,
        path: args.path,
        server: autoDetectServer(),
        ssl: ssl,
      };
    }

    setupSite(options)
      .then(() => {
        console.log("\n✅ Site setup completed successfully!");
        console.log(
          `\n🌐 Visit: http${options.ssl ? "s" : ""}://${options.domain}`
        );
        process.exit(0);
      })
      .catch((error) => {
        triggerError("\n❌ Setup failed:", error.message);
      });
    return;
  }

  showHelp();
}

main();
