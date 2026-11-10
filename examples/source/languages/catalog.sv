// Totals the sizes of a small catalog of entries every clock cycle.
typedef struct packed {
    logic [7:0]  id;
    logic [15:0] bytes;
} entry_t;

module catalog_total #(parameter int N = 2) (
    input  logic        clk,
    input  logic        rst_n,
    input  entry_t      entries [N],
    output logic [31:0] total
);
    logic [31:0] sum;

    always_comb begin
        sum = '0;
        foreach (entries[i]) sum += entries[i].bytes;
    end

    always_ff @(posedge clk or negedge rst_n) begin
        if (!rst_n) total <= '0;
        else        total <= sum;
    end
endmodule
