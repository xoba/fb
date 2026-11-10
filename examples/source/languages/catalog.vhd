-- A free-running counter that toggles an LED each time it wraps.
library ieee;
use ieee.std_logic_1164.all;

entity blinker is
    generic (LIMIT : natural := 12_000_000);
    port (clk, rst : in std_logic; led : out std_logic);
end entity blinker;

architecture rtl of blinker is
    signal count : natural range 0 to LIMIT := 0;
    signal state : std_logic := '0';
begin
    process (clk)
    begin
        if rising_edge(clk) then
            if rst = '1' then
                count <= 0;
                state <= '0';
            elsif count = LIMIT then
                count <= 0;
                state <= not state;
            else
                count <= count + 1;
            end if;
        end if;
    end process;
    led <= state;
end architecture rtl;
