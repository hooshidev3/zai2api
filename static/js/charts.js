// charts.js — Simple canvas charts (no external dependencies)

class SimpleChart {
    constructor(canvasId, maxPoints = 60) {
        this.canvas = document.getElementById(canvasId);
        if (!this.canvas) return;
        this.ctx = this.canvas.getContext('2d');
        this.data = [];
        this.maxPoints = maxPoints;
        this.resize();
        window.addEventListener('resize', () => this.resize());
    }

    resize() {
        if (!this.canvas) return;
        const rect = this.canvas.getBoundingClientRect();
        this.canvas.width = rect.width;
        this.canvas.height = 120;
    }

    push(value) {
        this.data.push(Math.max(0, value));
        if (this.data.length > this.maxPoints) this.data.shift();
        this.draw();
    }

    draw() {
        if (!this.ctx) return;
        const { ctx, canvas, data } = this;
        const w = canvas.width, h = canvas.height;
        ctx.clearRect(0, 0, w, h);

        if (data.length < 2) return;

        const max = Math.max(...data, 1);
        const step = w / (this.maxPoints - 1);

        // Fill gradient
        const grad = ctx.createLinearGradient(0, 0, 0, h);
        grad.addColorStop(0, 'rgba(88, 166, 255, 0.3)');
        grad.addColorStop(1, 'rgba(88, 166, 255, 0)');

        ctx.beginPath();
        ctx.moveTo(0, h);
        data.forEach((v, i) => {
            const x = i * step;
            const y = h - (v / max) * (h - 10);
            ctx.lineTo(x, y);
        });
        ctx.lineTo((data.length - 1) * step, h);
        ctx.closePath();
        ctx.fillStyle = grad;
        ctx.fill();

        // Line
        ctx.beginPath();
        ctx.strokeStyle = '#58a6ff';
        ctx.lineWidth = 2;
        data.forEach((v, i) => {
            const x = i * step;
            const y = h - (v / max) * (h - 10);
            if (i === 0) ctx.moveTo(x, y);
            else ctx.lineTo(x, y);
        });
        ctx.stroke();
    }
}

let reqChart, errChart;
let lastReqs = 0, lastErrs = 0;

document.addEventListener('DOMContentLoaded', () => {
    reqChart = new SimpleChart('chart-requests');
    errChart = new SimpleChart('chart-errors');
});

function updateCharts(kpis) {
    const reqDelta = kpis.total_requests - lastReqs;
    const errDelta = kpis.total_errors - lastErrs;
    lastReqs = kpis.total_requests;
    lastErrs = kpis.total_errors;
    if (reqChart) reqChart.push(reqDelta);
    if (errChart) errChart.push(errDelta);
}
