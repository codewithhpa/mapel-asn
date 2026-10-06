/**
 * Mazer Simple DataTable Auto Initializer & Bootstrap 5 Adapter
 * Provides sortable, searchable, and paginated tables identical to dist/table-datatable.html
 */
function initMazerTable(tableEl, customOptions = {}) {
    if (!tableEl || !(tableEl instanceof HTMLElement)) return null;
    if (tableEl.getAttribute('data-datatable-initialized')) return null;

    const defaultOptions = {
        searchable: true,
        fixedHeight: false,
        perPage: parseInt(tableEl.getAttribute('data-per-page')) || 5,
        perPageSelect: [5, 10, 25, 50, 100],
        labels: {
            placeholder: "Cari data...",
            searchTitle: "Cari di tabel",
            pageTitle: "Halaman {page}",
            perPage: "{select} entri per halaman",
            noRows: "Tidak ada data ditemukan",
            info: "Menampilkan {start} sampai {end} dari {rows} entri",
            noResults: "Tidak ada data yang cocok dengan pencarian"
        }
    };

    const options = Object.assign({}, defaultOptions, customOptions);
    const dataTable = new simpleDatatables.DataTable(tableEl, options);

    function adaptPageDropdown() {
        const selector = dataTable.wrapper.querySelector(".dataTable-selector");
        if (selector && selector.parentNode && selector.parentNode.tagName === "LABEL") {
            selector.parentNode.parentNode.insertBefore(selector, selector.parentNode);
            selector.classList.add("form-select", "form-select-sm", "d-inline-block", "w-auto");
        }
        const input = dataTable.wrapper.querySelector(".dataTable-input");
        if (input) {
            input.classList.add("form-control", "form-control-sm");
        }
    }

    function adaptPagination() {
        const paginations = dataTable.wrapper.querySelectorAll("ul.dataTable-pagination-list");
        for (const pagination of paginations) {
            pagination.classList.add("pagination", "pagination-primary", "mb-0");
        }

        const paginationLis = dataTable.wrapper.querySelectorAll("ul.dataTable-pagination-list li");
        for (const paginationLi of paginationLis) {
            paginationLi.classList.add("page-item");
        }

        const paginationLinks = dataTable.wrapper.querySelectorAll("ul.dataTable-pagination-list li a");
        for (const paginationLink of paginationLinks) {
            paginationLink.classList.add("page-link");
        }

        // Always show page 1 indicator even if only 1 page
        const navPagination = dataTable.wrapper.querySelector("nav.dataTable-pagination");
        if (navPagination) {
            let ul = navPagination.querySelector("ul.dataTable-pagination-list");
            if (!ul) {
                ul = document.createElement("ul");
                ul.className = "dataTable-pagination-list pagination pagination-primary mb-0";
                navPagination.appendChild(ul);
            }
            if (ul.children.length === 0) {
                ul.innerHTML = '<li class="page-item active"><span class="page-link">1</span></li>';
            }
        }
    }

    dataTable.on("datatable.init", () => {
        adaptPageDropdown();
        adaptPagination();
        tableEl.setAttribute('data-datatable-initialized', 'true');
    });

    dataTable.on("datatable.update", adaptPagination);
    dataTable.on("datatable.sort", adaptPagination);
    dataTable.on("datatable.page", adaptPagination);

    tableEl._mazerDT = dataTable;
    return dataTable;
}

// Auto init all tables with class 'table-datatable' or id 'table1' on DOMContentLoaded
document.addEventListener("DOMContentLoaded", function() {
    if (typeof simpleDatatables === "undefined") return;

    const tables = document.querySelectorAll(".table-datatable, #table1");
    tables.forEach(table => {
        initMazerTable(table);
    });
});

/**
 * Universal Client-Side Table Exporter to CSV / Excel
 * Formats data with UTF-8 BOM and semicolon delimiters for Indonesian Excel
 */
function exportTableToCSV(tableId, filename = 'Export_Data') {
    const table = document.getElementById(tableId);
    if (!table) return;

    let csv = [];
    const headers = [];
    const headerCols = table.querySelectorAll('thead th');
    const ignoreIndexes = [];

    headerCols.forEach((col, idx) => {
        const text = col.innerText.trim().replace(/\r?\n|\r/g, ' ');
        if (text.toLowerCase() === 'aksi') {
            ignoreIndexes.push(idx);
        } else {
            headers.push('"' + text.replace(/"/g, '""') + '"');
        }
    });
    csv.push(headers.join(';'));

    const rows = table.querySelectorAll('tbody tr');
    rows.forEach(row => {
        if (row.querySelector('td[colspan]')) return;

        const rowData = [];
        const cols = row.querySelectorAll('td');
        cols.forEach((col, idx) => {
            if (ignoreIndexes.includes(idx)) return;
            let text = col.innerText.trim().replace(/\r?\n|\r/g, ' ');
            rowData.push('"' + text.replace(/"/g, '""') + '"');
        });
        if (rowData.length > 0) {
            csv.push(rowData.join(';'));
        }
    });

    const csvContent = "\uFEFF" + csv.join("\r\n");
    const blob = new Blob([csvContent], { type: "text/csv;charset=utf-8;" });
    const url = URL.createObjectURL(blob);
    const link = document.createElement("a");
    link.setAttribute("href", url);
    const todayStr = new Date().toISOString().slice(0, 10);
    link.setAttribute("download", `${filename}_${todayStr}.csv`);
    document.body.appendChild(link);
    link.click();
    document.body.removeChild(link);
}

