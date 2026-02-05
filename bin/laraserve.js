#!/usr/bin/env node

const minimist = require("minimist");
const { setupSite } = require("../lib/setup");

function showHelp() {
  console.log(`
laraserve - Local site configuration generator

Usage:
  laraserve                    Run in interactive mode
  laraserve --domain <domain> --path <path> --server <apache|nginx> [--ssl]

Options:
  --domain <domain>    Domain name (e.g., example.test)
  --path <path>        Document root path (e.g., /var/www/example)
  --server <server>    Server type: apache or nginx
  --ssl                Enable SSL with mkcert (optional)
  --help               Show this help message

Example:
  laraserve --domain myapp.test --path /var/www/myapp --server nginx --ssl
  `);
  process.exit(0);
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
      type: "list",
      name: "server",
      message: "Select web server:",
      choices: [
        { name: "Nginx", value: "nginx" },
        { name: "Apache", value: "apache" },
      ],
    },
    {
      type: "confirm",
      name: "ssl",
      message: "Enable SSL with mkcert?",
      default: false,
    },
  ];

  const answers = await prompt(questions);
  return {
    domain: answers.domain.trim(),
    path: answers.path.trim(),
    server: answers.server,
    ssl: answers.ssl,
  };
}

async function main() {
  const args = minimist(process.argv.slice(2), {
    string: ["domain", "path", "server"],
    boolean: ["ssl", "help"],
    alias: {
      h: "help",
      d: "domain",
      p: "path",
      s: "server",
    },
  });

  if (args.help) {
    showHelp();
  }

  let options;

  if (!args.domain || !args.path || !args.server) {
    console.log("\n🚀 laraserve - Interactive Site Configuration\n");
    console.log("Missing required arguments. Starting interactive mode...\n");
    options = await promptForOptions();
  } else {
    options = {
      domain: args.domain,
      path: args.path,
      server: args.server.toLowerCase(),
      ssl: args.ssl || false,
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
      console.error("\n❌ Setup failed:", error.message);
      process.exit(1);
    });
}

main();
