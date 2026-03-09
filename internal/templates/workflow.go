package templates

const SharedWorkflow = `name: Deploy
on:
  push:
    branches: [main, master]
jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: docker/setup-buildx-action@v3
      - name: Build images
        run: docker compose -f docker-compose.yml build
      - name: Save images
        run: docker save $(docker compose config --images) | gzip > images.tar.gz
      - uses: webfactory/ssh-agent@v0.9.0
        with:
          ssh-private-key: ${{"{{"}} secrets.SSH_PRIVATE_KEY {{"}}"}}
      - name: Add host key
        run: ssh-keyscan -H ${{"{{"}} secrets.DEPLOY_HOST {{"}}"}} >> ~/.ssh/known_hosts
      - name: Deploy
        run: |
          scp docker-compose.yml ${{"{{"}} secrets.DEPLOY_USER {{"}}"}}@${{"{{"}} secrets.DEPLOY_HOST {{"}}"}}:~/docker-compose.yml
          scp images.tar.gz ${{"{{"}} secrets.DEPLOY_USER {{"}}"}}@${{"{{"}} secrets.DEPLOY_HOST {{"}}"}}:~/
          ssh ${{"{{"}} secrets.DEPLOY_USER {{"}}"}}@${{"{{"}} secrets.DEPLOY_HOST {{"}}"}} << 'EOF'
            docker load < ~/images.tar.gz
            rm ~/images.tar.gz
            docker compose up -d
          EOF
`
