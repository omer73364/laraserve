const hosts = require("./hosts");
const permissions = require("./permissions");
const server = require("./server");
const ssl = require("./ssl");
const php = require("./php");

module.exports = {
  ...hosts,
  ...permissions,
  ...server,
  ...ssl,
  ...php,
};
