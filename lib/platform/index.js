const { detectPlatform, Platform } = require("./detector");

let platformModule;

switch (detectPlatform()) {
  case Platform.LINUX:
    platformModule = require("./linux");
    break;
  case Platform.DARWIN:
    platformModule = require("./mac");
    break;
  case Platform.WINDOWS:
    platformModule = require("./windows");
    break;
  default:
    platformModule = require("./linux");
}

module.exports = {
  ...require("./detector"),
  ...platformModule,
};
