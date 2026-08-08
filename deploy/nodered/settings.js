module.exports = {
  uiPort: process.env.PORT || 1880,
  mqttReconnectTime: 15000,
  serialReconnectTime: 15000,
  debugMaxLength: 1000,
  functionGlobalContext: {},
  exportGlobalContextKeys: false,
  credentialSecret: process.env.NODERED_CREDENTIAL_SECRET || "crearts-nodered-dev",
  logging: {
    console: {
      level: "info",
      metrics: false,
      audit: false,
    },
  },
  editorTheme: {
    page: { title: "Crearts IoT — Node-RED" },
    header: { title: "Crearts Automation" },
  },
  // Allow dashboard iframe (platform UI on another port).
  httpAdminMiddleware: function (req, res, next) {
    res.setHeader("Content-Security-Policy", "frame-ancestors *");
    res.removeHeader("X-Frame-Options");
    next();
  },
};
