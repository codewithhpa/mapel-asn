// 1. Inisiasi Grafik Tren Presensi (Bar Chart)
var chartProfileVisit = null
var elProfileVisit = document.querySelector("#chart-profile-visit")

// Hitung label default minggu ini dengan tanggal (misal: Senin (24 Ags), Selasa (25 Ags)...)
function getDefaultWeeklyCategories() {
  var now = new Date()
  var day = now.getDay() // 0 = Minggu, 1 = Senin, ...
  var diffToMonday = day === 0 ? -6 : 1 - day
  var monday = new Date(now)
  monday.setDate(now.getDate() + diffToMonday)

  var dayNames = ["Senin", "Selasa", "Rabu", "Kamis", "Jumat"]
  var monthNames = ["Jan", "Feb", "Mar", "Apr", "Mei", "Jun", "Jul", "Ags", "Sep", "Okt", "Nov", "Des"]
  var cats = []

  for (var i = 0; i < 5; i++) {
    var d = new Date(monday)
    d.setDate(monday.getDate() + i)
    var dateNum = d.getDate() < 10 ? "0" + d.getDate() : d.getDate()
    var monthStr = monthNames[d.getMonth()]
    cats.push(dayNames[i] + " (" + dateNum + " " + monthStr + ")")
  }
  return cats
}

if (elProfileVisit) {
  var isDark = document.body.classList.contains('theme-dark') || document.documentElement.getAttribute('data-bs-theme') === 'dark'

  var optionsProfileVisit = {
    annotations: {
      position: "back",
    },
    dataLabels: {
      enabled: false,
    },
    chart: {
      type: "bar",
      height: 310,
      fontFamily: "'Plus Jakarta Sans', sans-serif",
      toolbar: {
        show: false,
      },
    },
    plotOptions: {
      bar: {
        horizontal: false,
        columnWidth: "42%",
        borderRadius: 5,
        borderRadiusApplication: "end",
      },
    },
    fill: {
      opacity: 1,
    },
    series: [
      {
        name: "Hadir Sah (Apel Pagi / Sore)",
        data: [0, 0, 0, 0, 0],
      },
      {
        name: "Terlambat / Temuan",
        data: [0, 0, 0, 0, 0],
      },
    ],
    colors: ["#10b981", "#ef4444"],
    grid: {
      borderColor: isDark ? "rgba(255, 255, 255, 0.06)" : "#f1f5f9",
      strokeDashArray: 4,
    },
    xaxis: {
      categories: getDefaultWeeklyCategories(),
      labels: {
        style: {
          colors: isDark ? "#94a3b8" : "#64748b",
          fontSize: "11.5px",
          fontFamily: "'Plus Jakarta Sans', sans-serif",
        },
      },
      axisBorder: {
        show: false,
      },
      axisTicks: {
        show: false,
      },
    },
    yaxis: {
      labels: {
        style: {
          colors: isDark ? "#94a3b8" : "#64748b",
          fontSize: "11.5px",
          fontFamily: "'Plus Jakarta Sans', sans-serif",
        },
      },
    },
    legend: {
      show: true,
      position: "bottom",
      fontSize: "11.5px",
      fontWeight: 500,
      itemMargin: {
        horizontal: 8,
        vertical: 4,
      },
      markers: {
        radius: 4,
      },
    },
    tooltip: {
      theme: isDark ? "dark" : "light",
    },
    noData: {
      text: "Memuat data presensi...",
      align: "center",
      verticalAlign: "middle",
      style: {
        fontSize: "12px",
        color: isDark ? "#94a3b8" : "#64748b",
      },
    },
  }

  chartProfileVisit = new ApexCharts(elProfileVisit, optionsProfileVisit)
  chartProfileVisit.render()
}

// 2. Inisiasi Grafik Distribusi Kehadiran (Donut Chart) - Data Riil dari API
var chartVisitorsProfile = null
var elVisitorsProfile = document.querySelector("#chart-visitors-profile")

if (elVisitorsProfile) {
  var isDark = document.body.classList.contains('theme-dark') || document.documentElement.getAttribute('data-bs-theme') === 'dark'
  
  var optionsVisitorsProfile = {
    series: [0, 0, 0, 0],
    labels: ["Tepat Waktu (Pagi)", "Terlambat (Pagi)", "Hadir Apel Sore", "Temuan Ketidaksesuaian"],
    colors: ["#10b981", "#f59e0b", "#3b82f6", "#ef4444"],
    chart: {
      type: "donut",
      width: "100%",
      height: 310,
      fontFamily: "'Plus Jakarta Sans', sans-serif",
    },
    stroke: {
      width: 3,
      colors: [isDark ? '#1e1e2d' : '#ffffff'],
    },
    dataLabels: {
      enabled: false,
    },
    legend: {
      position: "bottom",
      fontSize: "11.5px",
      fontWeight: 500,
      itemMargin: {
        horizontal: 6,
        vertical: 3,
      },
      markers: {
        radius: 4,
      },
    },
    plotOptions: {
      pie: {
        donut: {
          size: "70%",
          labels: {
            show: true,
            name: {
              show: true,
              fontSize: "12px",
              fontWeight: 600,
              color: isDark ? "#94a3b8" : "#64748b",
              offsetY: -3,
            },
            value: {
              show: true,
              fontSize: "22px",
              fontWeight: 800,
              fontFamily: "monospace",
              color: isDark ? "#f8fafc" : "#0f172a",
              offsetY: 5,
              formatter: function (val) {
                return val
              },
            },
            total: {
              show: true,
              label: "Total Rekap",
              fontSize: "11px",
              fontWeight: 600,
              color: isDark ? "#94a3b8" : "#64748b",
              formatter: function (w) {
                return w.globals.seriesTotals.reduce(function (a, b) {
                  return a + b
                }, 0)
              },
            },
          },
        },
      },
    },
    noData: {
      text: "Memuat data presensi...",
      align: "center",
      verticalAlign: "middle",
      style: {
        fontSize: "12px",
        color: isDark ? "#94a3b8" : "#64748b",
      },
    },
  }
  chartVisitorsProfile = new ApexCharts(elVisitorsProfile, optionsVisitorsProfile)
  chartVisitorsProfile.render()
}

// Fungsi Fetch Donut Chart dari API /api/presensi/distribution (sumber tunggal donut)
var donutBaseColors = ["#10b981", "#f59e0b", "#3b82f6", "#ef4444"]
var donutBaseLabels = ["Tepat Waktu (Pagi)", "Terlambat (Pagi)", "Hadir Apel Sore", "Temuan Ketidaksesuaian"]
function loadDonutData() {
  if (!chartVisitorsProfile) return
  var search = window.location.search || ""
  fetch("/api/presensi/distribution" + search)
    .then(function (res) {
      if (!res.ok) throw new Error("Status " + res.status)
      return res.json()
    })
    .then(function (data) {
      if (data && data.series) {
        var allZero = data.series.every(function (v) { return v === 0 })
        if (allZero) {
          // Data benar kosong: ring kosong + Total Rekap 0 (jujur; bukan slice
          // palsu bernilai 1 yang dulu bikin total tampil "1")
          chartVisitorsProfile.updateOptions({
            labels: data.labels || donutBaseLabels,
            colors: donutBaseColors.slice(),
          })
          chartVisitorsProfile.updateSeries([0, 0, 0, 0])
        } else {
          chartVisitorsProfile.updateOptions({
            labels: data.labels || donutBaseLabels,
            colors: donutBaseColors.slice(),
          })
          chartVisitorsProfile.updateSeries(data.series)
        }
      }
    })
    .catch(function (err) {
      console.error("Gagal memuat data distribusi presensi dari API:", err)
    })
}

// 3. Event Listener Interaktif: Tombol Periode Daily / Weekly / Monthly / Annual
document.addEventListener("DOMContentLoaded", function () {
  var periodButtons = document.querySelectorAll("#chartPeriodGroup button")
  var periodLabel = document.getElementById("chartPeriodLabel")

  var chartLoading = false

  function setPeriodButtonsEnabled(enabled) {
    periodButtons.forEach(function (b) {
      b.disabled = !enabled
      b.style.opacity = enabled ? "" : "0.55"
      b.style.pointerEvents = enabled ? "" : "none"
    })
  }

  function loadChartPeriod(period) {
    if (!chartProfileVisit || chartLoading) return
    chartLoading = true
    setPeriodButtonsEnabled(false)
    var prevLabel = periodLabel ? periodLabel.textContent : ""
    if (periodLabel) periodLabel.textContent = "Memuat data ..."

    fetch("/api/presensi/chart?period=" + period)
      .then(function (res) {
        if (!res.ok) throw new Error("Status " + res.status)
        return res.json()
      })
      .then(function (data) {
        if (periodLabel && data.period_label) {
          periodLabel.textContent = data.period_label
        }
        chartProfileVisit.updateOptions({
          xaxis: {
            categories: data.categories,
          },
        })
        chartProfileVisit.updateSeries(data.series)
        // Donut punya sumber tunggal (loadDonutData, ikut filter halaman) agar tidak balapan.
      })
      .catch(function (err) {
        console.error("Gagal memuat data periode chart:", err)
        if (periodLabel) {
          periodLabel.textContent = (prevLabel ? prevLabel + " — " : "") + "Gagal memuat, coba lagi"
        }
      })
      .finally(function () {
        chartLoading = false
        setPeriodButtonsEnabled(true)
      })
  }

  // Sinkronkan warna chart saat dark/light mode diganti (dark.js memancarkan theme-changed)
  function chartIsDark() {
    return document.documentElement.getAttribute("data-bs-theme") === "dark" ||
      document.body.classList.contains("theme-dark")
  }
  function applyChartTheme() {
    var dark = chartIsDark()
    var label = dark ? "#94a3b8" : "#64748b"
    if (chartProfileVisit) {
      chartProfileVisit.updateOptions({
        grid: { borderColor: dark ? "rgba(255, 255, 255, 0.06)" : "#f1f5f9" },
        xaxis: { labels: { style: { colors: label } } },
        yaxis: { labels: { style: { colors: label } } },
        tooltip: { theme: dark ? "dark" : "light" },
      })
    }
    if (chartVisitorsProfile) {
      chartVisitorsProfile.updateOptions({
        stroke: { colors: [dark ? "#1e1e2d" : "#ffffff"] },
        plotOptions: { pie: { donut: { labels: {
          name: { color: label },
          value: { color: dark ? "#f8fafc" : "#0f172a" },
          total: { color: label },
        } } } },
      })
    }
  }
  window.addEventListener("theme-changed", applyChartTheme)

  // Tentukan default periode berdasarkan URL.
  // TV default Mingguan (display lobby fokus minggu berjalan);
  // dashboard utama default Tahunan. Pemetaan ?period= tetap sama di keduanya
  // agar filter log halaman ikut menggerakkan chart.
  var urlParams = new URLSearchParams(window.location.search)
  var globalPeriod = urlParams.get("period")
  var isTVPage = !!document.getElementById("tv")
  var initialChartPeriod = isTVPage ? "weekly" : "annual"

  if (globalPeriod === "today" || globalPeriod === "date") {
    initialChartPeriod = "daily"
  } else if (globalPeriod === "this_week") {
    initialChartPeriod = "weekly"
  } else if (globalPeriod === "this_month" || globalPeriod === "month") {
    initialChartPeriod = "monthly"
  } else if (globalPeriod === "all" || globalPeriod === "annual") {
    initialChartPeriod = "annual"
  }

  // Set tombol aktif sesuai initialChartPeriod (pakai .active milik .segmented-control)
  periodButtons.forEach(function (b) {
    b.classList.toggle("active", b.getAttribute("data-period") === initialChartPeriod)
  })

  // Load data awal dari API
  loadChartPeriod(initialChartPeriod)
  loadDonutData()

  periodButtons.forEach(function (btn) {
    btn.addEventListener("click", function () {
      var period = this.getAttribute("data-period")

      // Update state tombol aktif (.segmented-control .btn.active)
      periodButtons.forEach(function (b) { b.classList.remove("active") })
      this.classList.add("active")

      loadChartPeriod(period)
    })
  })
})
