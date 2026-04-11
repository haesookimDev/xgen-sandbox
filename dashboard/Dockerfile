FROM node:22-alpine AS builder
WORKDIR /app

COPY package.json package-lock.json* ./
RUN npm install --frozen-lockfile 2>/dev/null || npm install

COPY . .
RUN npm run build

FROM node:22-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production

RUN apk upgrade --no-cache && \
    PICO_DIR=$(find /usr/local/lib/node_modules/npm -type d -name picomatch | head -1) && \
    wget -qO /tmp/p.tgz https://registry.npmjs.org/picomatch/-/picomatch-4.0.4.tgz && \
    rm -rf "$PICO_DIR"/* && \
    tar xzf /tmp/p.tgz --strip-components=1 -C "$PICO_DIR" && \
    rm /tmp/p.tgz

RUN addgroup --system --gid 1001 nodejs && \
    adduser --system --uid 1001 nextjs

COPY --from=builder --chown=nextjs:nodejs /app/.next/standalone ./
COPY --from=builder --chown=nextjs:nodejs /app/.next/static ./.next/static
COPY --from=builder --chown=nextjs:nodejs /app/public ./public

USER nextjs
EXPOSE 3000
ENV PORT=3000
ENV HOSTNAME="0.0.0.0"

CMD ["node", "server.js"]
