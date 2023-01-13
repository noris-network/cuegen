package kube

configMap: conf: data: {
	"server.conf": """

      server {
        listen         0.0.0.0:8080;
        server_name    \(values.myapp.hostname);
        root           /app;
        index          index.htm index.html;
        stub_status    /stub_status;
      }

      """
}
