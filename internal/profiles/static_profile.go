package profiles

func init() {
	Register(&Profile{
		Name:          "static",
		Description:   "Nginx serving static files with CDN headers. For landing pages and docs.",
		DefaultCPU:    "0.5",
		DefaultMemory: "256M",
		Services: []Service{
			{Name: "nginx", Image: "nginx:alpine", Description: "Nginx web server", Required: true},
		},
		Compose: `services:
  nginx:
    image: nginx:alpine
    container_name: {{.Name}}-nginx
    restart: always
    deploy:
      resources:
        limits:
          cpus: '{{.CPULimit}}'
          memory: {{.MemoryLimit}}
    volumes:
      - ./public:/usr/share/nginx/html:ro
      - ./nginx.conf:/etc/nginx/conf.d/default.conf:ro
    labels:
      - "traefik.enable=true"
      - "traefik.http.routers.{{.Name}}.rule=Host(` + "`{{.Domain}}`" + `)"
      - "traefik.http.routers.{{.Name}}.entrypoints=websecure"
      - "traefik.http.routers.{{.Name}}.tls.certresolver=myresolver"
      - "traefik.http.routers.{{.Name}}.tls=true"
      - "traefik.http.services.{{.Name}}.loadbalancer.server.port=80"
      - "traefik.http.middlewares.{{.Name}}-headers.headers.customresponseheaders.Cache-Control=public, max-age=31536000"
    networks:
      - default

networks:
  default:
    name: traefik_default
    external: true
`,
		EnvTemplate: `# Static site - no environment variables needed
`,
		Nginx: `server {
    listen 80;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;

    # Gzip compression
    gzip on;
    gzip_types text/plain text/css application/json application/javascript text/xml application/xml text/javascript image/svg+xml;
    gzip_min_length 1000;

    # Cache static assets
    location ~* \.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot)$ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }

    # SPA fallback
    location / {
        try_files $uri $uri/ /index.html;
    }

    # Security headers
    add_header X-Frame-Options "SAMEORIGIN" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;
}
`,
	})
}
